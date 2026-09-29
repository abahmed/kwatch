package kube_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func runActive(
	t *testing.T, cfg kube.ActiveProbeConfig,
) map[knowledge.EntityID]knowledge.Fact {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []knowledge.Fact, 1)
	cfg.Submit = func(_ context.Context, f ...knowledge.Fact) {
		got <- f
	}
	if cfg.Now == nil {
		cfg.Now = fixedTime
	}
	done := make(chan struct{})
	go func() { kube.NewActiveProber(cfg).Run(ctx); close(done) }()
	facts := <-got
	cancel()
	<-done
	out := map[knowledge.EntityID]knowledge.Fact{}
	for _, f := range facts {
		out[f.Entity] = f
	}
	return out
}

func endpoint(name string) knowledge.EntityID {
	return knowledge.NewEntityID(kube.KindEndpoint, "", name)
}

func TestActiveProberChecksHTTPTCPAndDNS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/teapot" {
				w.WriteHeader(http.StatusTeapot)
			}
			if r.URL.Path == "/down" {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
	defer srv.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	deadAddr := closed.Addr().String()
	require.NoError(t, closed.Close())

	facts := runActive(t, kube.ActiveProbeConfig{
		Targets: []kube.ProbeTarget{
			{Name: "ok", URL: srv.URL, LatencyWarningMs: 100,
				LatencyCriticalMs: 500},
			{Name: "teapot", URL: srv.URL + "/teapot",
				ExpectedStatus: http.StatusTeapot},
			{Name: "down", URL: srv.URL + "/down"},
			{Name: "badurl", URL: "http://[::1"},
			{Name: "tcp", Address: ln.Addr().String()},
			{Name: "tcpdead", Address: deadAddr},
			{Name: "dns", Host: "example.test"},
			{Name: "empty"},
		},
		HTTPClient: srv.Client(),
		Resolver:   fakeResolver{},
		Interval:   time.Minute, FailureThreshold: 2,
	})
	for name, want := range map[string]bool{
		"ok": true, "teapot": true, "down": false, "badurl": false,
		"tcp": true, "tcpdead": false, "dns": true, "empty": false,
	} {
		assert.Equal(t, want, healthy(t, facts[endpoint(name)]), name)
	}
	ok := facts[endpoint("ok")]
	assert.Equal(t, "active-probe", ok.Source)
	v, _ := ok.Attributes[kube.AttrFailureDuration].AsNumber()
	assert.InDelta(t, 120, v, 0.001)
	v, _ = ok.Attributes[kube.AttrLatencyWarnMS].AsNumber()
	assert.InDelta(t, 100, v, 0.001)
	v, _ = ok.Attributes[kube.AttrLatencyCritMS].AsNumber()
	assert.InDelta(t, 500, v, 0.001)
	assert.Contains(t,
		facts[endpoint("down")].Attributes[kube.AttrProbeError].AsText(),
		"500")
}

func TestActiveProberProbesServicePorts(t *testing.T) {
	model := knowledge.NewModel(knowledge.Options{})
	add := func(ns, name string, ports string) knowledge.EntityID {
		id := knowledge.NewEntityID(kube.KindService, ns, name)
		attrs := map[string]knowledge.Value{}
		if ports != "" {
			attrs[kube.AttrPorts] = knowledge.Text(ports)
		}
		_, err := model.Apply(knowledge.Fact{
			Kind: knowledge.Observed, Source: kube.FactSource,
			At: fixedTime(), Entity: id, Attributes: attrs,
		})
		require.NoError(t, err)
		return id
	}
	// Unresolvable in-cluster names make the dial fail quickly; the fact
	// is what matters, not the outcome.
	web := add("ns", "web", "80/TCP->8080")
	add("skipped", "web", "80/TCP->8080")
	add("ns", "noports", "")
	add("ns", "textport", "http/TCP->80")
	add("ns", "bare", "443")
	facts := runActive(t, kube.ActiveProbeConfig{
		AutoServices: true,
		Excluded:     map[string]bool{"skipped": true},
		Model:        model,
		Timeout:      200 * time.Millisecond,
	})
	assert.Contains(t, facts, web)
	assert.Contains(t, facts,
		knowledge.NewEntityID(kube.KindService, "ns", "bare"))
	assert.Len(t, facts, 2)
	assert.Equal(t, "active-probe", facts[web].Source)
}
