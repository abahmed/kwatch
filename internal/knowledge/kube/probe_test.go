package kube_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

type fakeResolver struct{ err error }

func (r fakeResolver) LookupHost(
	context.Context, string,
) ([]string, error) {
	return []string{"10.0.0.1"}, r.err
}

func leaseJSON(name string, renewed time.Time) string {
	return fmt.Sprintf(`{"apiVersion":"coordination.k8s.io/v1",`+
		`"kind":"Lease","metadata":{"name":%q,"namespace":"kube-system"},`+
		`"spec":{"renewTime":%q}}`, name,
		renewed.UTC().Format("2006-01-02T15:04:05.000000Z"))
}

func probeHandler(readyz string, now time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/readyz":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(readyz))
		case strings.HasSuffix(r.URL.Path, "/kube-scheduler"):
			_, _ = w.Write([]byte(leaseJSON("kube-scheduler", now)))
		case strings.HasSuffix(r.URL.Path, "/kube-controller-manager"):
			_, _ = w.Write([]byte(leaseJSON(
				"kube-controller-manager", now.Add(-10*time.Minute))))
		default:
			http.NotFound(w, r)
		}
	})
}

func runProbe(
	t *testing.T, cfg kube.ProbeConfig,
) map[knowledge.EntityID]knowledge.Fact {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []knowledge.Fact, 1)
	cfg.Submit = func(_ context.Context, f ...knowledge.Fact) {
		got <- f
	}
	done := make(chan struct{})
	go func() { kube.NewProber(cfg).Run(ctx); close(done) }()
	facts := <-got
	cancel()
	<-done
	out := map[knowledge.EntityID]knowledge.Fact{}
	for _, f := range facts {
		out[f.Entity] = f
	}
	return out
}

func healthy(t *testing.T, f knowledge.Fact) bool {
	t.Helper()
	v, ok := f.Attributes[kube.AttrHealthy].AsBool()
	require.True(t, ok)
	return v
}

func TestProberReportsControlPlaneAndDNS(t *testing.T) {
	now := fixedTime()
	report := "[+]ping ok\n[+]etcd ok\nreadyz check passed\n"
	facts := runProbe(t, kube.ProbeConfig{
		Client:   restClient(t, probeHandler(report, now)),
		Resolver: fakeResolver{},
		Now:      func() time.Time { return now },
	})
	assert.True(t, healthy(t, facts[kube.APIServer]))
	assert.True(t, healthy(t, facts[kube.Etcd]))
	assert.True(t, healthy(t, facts[kube.ClusterDNS]))
	assert.True(t, healthy(t, facts[kube.Scheduler]))
	stale := facts[kube.ControllerManager]
	assert.False(t, healthy(t, stale))
	assert.Contains(t, stale.Attributes[kube.AttrProbeError].AsText(),
		"not renewed")
}

func TestProberReportsEtcdAndDNSFailures(t *testing.T) {
	now := fixedTime()
	report := "[+]ping ok\n[-]etcd failed: reason withheld\n"
	facts := runProbe(t, kube.ProbeConfig{
		Client:   restClient(t, probeHandler(report, now)),
		Resolver: fakeResolver{err: errors.New("no such host")},
		Now:      func() time.Time { return now },
	})
	assert.False(t, healthy(t, facts[kube.Etcd]))
	assert.False(t, healthy(t, facts[kube.ClusterDNS]))
	assert.Equal(t, "no such host",
		facts[kube.ClusterDNS].Attributes[kube.AttrProbeError].AsText())
}

func TestProberFlagsUnreachableAPIServer(t *testing.T) {
	now := fixedTime()
	facts := runProbe(t, kube.ProbeConfig{
		Client:   restClient(t, http.NotFoundHandler()),
		Resolver: fakeResolver{},
		Now:      func() time.Time { return now },
	})
	assert.False(t, healthy(t, facts[kube.APIServer]))
	_, hasEtcd := facts[kube.Etcd]
	assert.False(t, hasEtcd)
	_, hasLease := facts[kube.Scheduler]
	assert.False(t, hasLease)
}
