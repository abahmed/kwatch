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

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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
		case r.URL.Path == "/version":
			_, _ = w.Write([]byte(
				`{"major":"1","minor":"29+","gitVersion":"v1.29.3-eks-1"}`))
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
) map[inventory.EntityID]inventory.Observation {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []inventory.Observation, 1)
	cfg.Submit = func(_ context.Context, f ...inventory.Observation) {
		got <- f
	}
	done := make(chan struct{})
	go func() { kube.NewProber(cfg).Run(ctx); close(done) }()
	observations := <-got
	cancel()
	<-done
	out := map[inventory.EntityID]inventory.Observation{}
	for _, f := range observations {
		out[f.Entity] = f
	}
	return out
}

func healthy(t *testing.T, f inventory.Observation) bool {
	t.Helper()
	v, ok := f.Attributes[kube.AttrHealthy].AsBool()
	require.True(t, ok)
	return v
}

func TestProberReportsControlPlaneAndDNS(t *testing.T) {
	now := fixedTime()
	report := "[+]ping ok\n[+]etcd ok\nreadyz check passed\n"
	observations := runProbe(t, kube.ProbeConfig{
		Client:   restClient(t, probeHandler(report, now)),
		Resolver: fakeResolver{},
		Now:      func() time.Time { return now },
	})
	assert.True(t, healthy(t, observations[kube.APIServer]))
	assert.True(t, healthy(t, observations[kube.Etcd]))
	assert.True(t, healthy(t, observations[kube.ClusterDNS]))
	assert.True(t, healthy(t, observations[kube.Scheduler]))
	stale := observations[kube.ControllerManager]
	assert.False(t, healthy(t, stale))
	assert.Contains(t, stale.Attributes[kube.AttrProbeError].AsText(),
		"not renewed")
}

func TestProberRecordsServerVersion(t *testing.T) {
	now := fixedTime()
	observations := runProbe(t, kube.ProbeConfig{
		Client:   restClient(t, probeHandler("[+]etcd ok\n", now)),
		Resolver: fakeResolver{},
		Now:      func() time.Time { return now },
	})
	attrs := observations[kube.APIServer].Attributes
	assert.Equal(t, "v1.29.3-eks-1", attrs[kube.AttrServerVersion].AsText())
	minor, ok := attrs[kube.AttrServerMinor].AsNumber()
	assert.True(t, ok)
	assert.Equal(t, 29.0, minor)
}

func TestProberReportsEtcdAndDNSFailures(t *testing.T) {
	now := fixedTime()
	report := "[+]ping ok\n[-]etcd failed: reason withheld\n"
	observations := runProbe(t, kube.ProbeConfig{
		Client:   restClient(t, probeHandler(report, now)),
		Resolver: fakeResolver{err: errors.New("no such host")},
		Now:      func() time.Time { return now },
	})
	assert.False(t, healthy(t, observations[kube.Etcd]))
	assert.False(t, healthy(t, observations[kube.ClusterDNS]))
	assert.Equal(t, "no such host",
		observations[kube.ClusterDNS].Attributes[kube.AttrProbeError].AsText())
}

func TestProberFlagsUnreachableAPIServer(t *testing.T) {
	now := fixedTime()
	observations := runProbe(t, kube.ProbeConfig{
		Client:   restClient(t, http.NotFoundHandler()),
		Resolver: fakeResolver{},
		Now:      func() time.Time { return now },
	})
	assert.False(t, healthy(t, observations[kube.APIServer]))
	_, hasEtcd := observations[kube.Etcd]
	assert.False(t, hasEtcd)
	_, hasLease := observations[kube.Scheduler]
	assert.False(t, hasLease)
}

func TestParseMinorReadsDistributionVersions(t *testing.T) {
	cases := map[string]int{
		"v1.29.3": 29, "v1.30.1-gke.5": 30, "1.28.0": 28, "v1.27+": 27,
	}
	for version, want := range cases {
		got, ok := kube.ParseMinor(version)
		assert.True(t, ok, version)
		assert.Equal(t, want, got, version)
	}
	_, ok := kube.ParseMinor("unknown")
	assert.False(t, ok)
}
