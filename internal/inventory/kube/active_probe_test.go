package kube_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func runActive(
	t *testing.T, cfg kube.ActiveProbeConfig,
) map[inventory.EntityID]inventory.Observation {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []inventory.Observation, 1)
	cfg.Submit = func(_ context.Context, f ...inventory.Observation) {
		got <- f
	}
	if cfg.Now == nil {
		cfg.Now = fixedTime
	}
	done := make(chan struct{})
	go func() { kube.NewActiveProber(cfg).Run(ctx); close(done) }()
	observations := <-got
	cancel()
	<-done
	out := map[inventory.EntityID]inventory.Observation{}
	for _, f := range observations {
		out[f.Entity] = f
	}
	return out
}

func endpoint(name string) inventory.EntityID {
	return inventory.CoreID(kube.KindEndpoint, "", name)
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

	observations := runActive(t, kube.ActiveProbeConfig{
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
		assert.Equal(t, want, healthy(t, observations[endpoint(name)]), name)
	}
	ok := observations[endpoint("ok")]
	assert.Equal(t, "active-probe", ok.Source)
	v, _ := ok.Attributes[kube.AttrFailureDuration].AsNumber()
	assert.InDelta(t, 120, v, 0.001)
	v, _ = ok.Attributes[kube.AttrLatencyWarnMS].AsNumber()
	assert.InDelta(t, 100, v, 0.001)
	v, _ = ok.Attributes[kube.AttrLatencyCritMS].AsNumber()
	assert.InDelta(t, 500, v, 0.001)
	assert.Contains(t,
		observations[endpoint("down")].Attributes[kube.AttrProbeError].AsText(),
		"500")
}

func TestActiveProberProbesServicePorts(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	add := func(ns, name string, ports string) inventory.EntityID {
		id := inventory.CoreID(kube.KindService, ns, name)
		attrs := map[string]inventory.Value{}
		if ports != "" {
			attrs[kube.AttrPorts] = inventory.Text(ports)
		}
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: id, Attributes: attrs,
		})
		require.NoError(t, err)
		return id
	}
	var dialed []string
	var mu sync.Mutex
	refuse := func(
		_ context.Context, _, address string,
	) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		dialed = append(dialed, address)
		return nil, errors.New("connection refused")
	}
	web := add("ns", "web", "80/TCP->8080")
	add("skipped", "web", "80/TCP->8080")
	add("ns", "noports", "")
	add("ns", "textport", "http/TCP->80")
	add("ns", "bare", "443")
	observations := runActive(t, kube.ActiveProbeConfig{
		AutoServices: true,
		Excluded:     map[string]bool{"skipped": true},
		Model:        model,
		Timeout:      200 * time.Millisecond,
		Dial:         refuse,
	})
	assert.Contains(t, observations, web)
	mu.Lock()
	assert.ElementsMatch(t, []string{"web.ns.svc:80", "bare.ns.svc:443"},
		dialed)
	mu.Unlock()
	assert.Contains(t, observations,
		inventory.CoreID(kube.KindService, "ns", "bare"))
	assert.Len(t, observations, 2)
	assert.Equal(t, "active-probe-service", observations[web].Source)
}

func TestActiveProberLateServiceProbeDoesNotResurrect(t *testing.T) {
	model := inventory.NewModel(inventory.Options{
		EnrichmentSources: kube.EnrichmentSources(),
	})
	svc := inventory.CoreID(kube.KindService, "ns", "web")
	apply := func(o inventory.Observation) {
		t.Helper()
		_, err := model.Apply(o)
		require.NoError(t, err)
	}
	apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: fixedTime(), Entity: svc,
		Attributes: map[string]inventory.Value{
			kube.AttrPorts: inventory.Text("80/TCP->8080"),
		},
	})
	refuse := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}
	observations := runActive(t, kube.ActiveProbeConfig{
		Targets:      []kube.ProbeTarget{{Name: "db", Address: "db:5432"}},
		AutoServices: true, Model: model, Dial: refuse,
	})
	require.Contains(t, observations, svc)

	// The Service is deleted while the probe round was still running.
	apply(inventory.Observation{
		Kind: inventory.Gone, Source: kube.ObservationSource,
		At: fixedTime(), Entity: svc,
	})
	for _, o := range observations {
		apply(o)
	}

	assert.False(t, model.Exists(svc), "deleted Service resurrected")
	assert.True(t, model.Exists(endpoint("db")),
		"configured probe target must still create its endpoint")
}

// With autoDependencies on, every external endpoint a pod's environment
// names is dialled once, and the result lands on the endpoint entity.
func TestActiveProberProbesPodDependencies(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	db := inventory.CoreID(kube.KindExternalEndpoint, "", "db.example.com:5432")
	for _, name := range []string{"api-a", "api-b"} {
		pod := inventory.CoreID(kube.KindPod, "shop", name)
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: pod,
			Attributes: map[string]inventory.Value{},
		})
		require.NoError(t, err)
		_, err = model.Apply(inventory.Observation{
			Kind: inventory.Related, Source: kube.ObservationSource,
			At: fixedTime(), Entity: pod, Relation: inventory.Calls,
			Targets: []inventory.EntityID{db},
		})
		require.NoError(t, err)
	}
	var dialed []string
	var mu sync.Mutex
	refuse := func(_ context.Context, _, address string) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		dialed = append(dialed, address)
		return nil, errors.New("connection refused")
	}

	observations := runActive(t, kube.ActiveProbeConfig{
		AutoDependencies: true, Model: model,
		Timeout: 200 * time.Millisecond, Dial: refuse,
	})

	mu.Lock()
	assert.Equal(t, []string{"db.example.com:5432"}, dialed)
	mu.Unlock()
	require.Contains(t, observations, db)
	assert.Equal(t, "active-probe-dependency", observations[db].Source)
	healthy, _ := observations[db].Attributes[kube.AttrHealthy].AsBool()
	assert.False(t, healthy)
}

// An HTTPS target records the expiry of the certificate it served, so the
// certificate check covers what clients see.
func TestActiveProberRecordsServedCertificateExpiry(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer srv.Close()

	observations := runActive(t, kube.ActiveProbeConfig{
		Targets:    []kube.ProbeTarget{{Name: "tls", URL: srv.URL}},
		HTTPClient: srv.Client(), Resolver: fakeResolver{},
		Interval: time.Minute, FailureThreshold: 2,
	})

	got := observations[endpoint("tls")]
	assert.True(t, healthy(t, got))
	expiry, ok := got.Attributes[kube.AttrCertExpiry]
	require.True(t, ok, "expiry recorded")
	at := expiry.AsTime()
	assert.Equal(t, srv.Certificate().NotAfter.UTC(), at.UTC())
}
