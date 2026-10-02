package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
)

type recordedStatus struct {
	state, reason string
	available     bool
}

type fakeStatusSink struct {
	mu       sync.Mutex
	statuses map[string]recordedStatus
	calls    int
}

func (f *fakeStatusSink) SetComponentStatus(
	name, state, reason string, available bool,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statuses == nil {
		f.statuses = map[string]recordedStatus{}
	}
	f.statuses[name] = recordedStatus{state, reason, available}
	f.calls++
}

func (f *fakeStatusSink) get(name string) recordedStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statuses[name]
}

type fakeAvailability struct {
	mu       sync.Mutex
	statuses []kube.SourceStatus
}

func (f *fakeAvailability) set(statuses ...kube.SourceStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = statuses
}

func (f *fakeAvailability) Unavailable() []kube.SourceStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]kube.SourceStatus(nil), f.statuses...)
}

func TestSourceHealthReportsAndRecoversUnavailableResource(t *testing.T) {
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	source.set(kube.SourceStatus{
		Resource: "secrets", Reason: kube.ReasonPermissionDenied,
	})
	health := newSourceHealth(sink, source)
	before := metrics.DefaultRegistry().SourceUnavailable.Load()

	health.report()
	health.report()

	require.Equal(t, recordedStatus{
		"degraded", "optional_permission_denied", false,
	}, sink.get("source-secrets"))
	require.Equal(t, 1, sink.calls, "unchanged status republished")
	require.Equal(t, before+1,
		metrics.DefaultRegistry().SourceUnavailable.Load())

	source.set()
	health.report()

	require.Equal(t, recordedStatus{"running", "", true},
		sink.get("source-secrets"))
}

func TestSourceHealthReasonKeepsVocabulary(t *testing.T) {
	cases := map[string]kube.SourceStatus{
		"permission_denied": {Required: true,
			Reason: kube.ReasonPermissionDenied},
		"optional_api_unavailable": {Reason: kube.ReasonAPIUnavailable},
		"api_unavailable": {Required: true,
			Reason: kube.ReasonAPIUnavailable},
		"cache_sync_timeout": {Reason: kube.ReasonSyncTimeout},
		"cache_sync_pending": {Reason: kube.ReasonSyncPending},
		"cache_sync_failed":  {Reason: kube.ReasonSyncFailed},
	}
	for want, status := range cases {
		require.Equal(t, want, sourceHealthReason(status))
	}
}

func TestSourceHealthRunStopsOnCancellation(t *testing.T) {
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	health := newSourceHealth(sink, source)
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() { health.run(ctx, tick); close(done) }()
	source.set(kube.SourceStatus{
		Resource: "ingresses", Group: "networking.k8s.io",
		Reason: kube.ReasonSyncTimeout,
	})

	tick <- time.Time{}
	tick <- time.Time{} // accepted only after the first report finished
	cancel()
	<-done

	require.Equal(t, "cache_sync_timeout",
		sink.get("source-ingresses.networking.k8s.io").reason)
}

func newReadinessHealth(
	t *testing.T, source *fakeAvailability,
) (*sourceHealth, func() bool) {
	t.Helper()
	server, readiness := newTestReadiness()
	readiness.begin(1, false)
	readiness.setCurrent("state", true)
	health := newSourceHealth(&fakeStatusSink{}, source).
		withReadiness(func(ready bool) {
			readiness.setCurrent("pipeline", ready)
		})
	return health, server.Ready
}

func TestSourceHealthRequiredLossWithdrawsReadiness(t *testing.T) {
	source := &fakeAvailability{}
	health, ready := newReadinessHealth(t, source)
	health.report()
	require.True(t, ready())

	source.set(kube.SourceStatus{
		Resource: "pods", Required: true,
		Reason: kube.ReasonPermissionDenied,
	})
	health.report()
	require.False(t, ready())

	source.set()
	health.report()
	require.True(t, ready())
}

func TestSourceHealthOptionalLossKeepsReadiness(t *testing.T) {
	source := &fakeAvailability{}
	health, ready := newReadinessHealth(t, source)
	source.set(kube.SourceStatus{
		Resource: "secrets", Reason: kube.ReasonPermissionDenied,
	})
	health.report()
	require.True(t, ready())
}

func TestSourceHealthStaleReportCannotRestoreReadiness(t *testing.T) {
	source := &fakeAvailability{}
	server, readiness := newTestReadiness()
	readiness.begin(1, false)
	readiness.setCurrent("state", true)
	health := newSourceHealth(&fakeStatusSink{}, source).
		withReadiness(func(ready bool) {
			readiness.setCurrent("pipeline", ready)
		})

	readiness.end(1)
	health.report()
	require.False(t, server.Ready())
}
