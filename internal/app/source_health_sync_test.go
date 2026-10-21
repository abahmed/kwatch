package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A required kind that is forbidden never syncs. Its reason must reach
// /health at the first tick, while the pipeline is still waiting, and
// readiness must stay with the startup hold until the sync finishes.
func TestSourceHealthReportsBeforeTheSourcesSync(t *testing.T) {
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	source.set(kube.SourceStatus{
		Resource: "pods", Required: true,
		Reason: kube.ReasonPermissionDenied,
	})
	var readyCalls atomic.Int32
	health := newSourceHealth(sink, source).
		withReadiness(func(bool) { readyCalls.Add(1) })
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	synced := make(chan struct{})
	done := make(chan struct{})
	go func() { health.run(ctx, tick, synced); close(done) }()

	tick <- time.Time{}
	tick <- time.Time{} // accepted only after the first report finished

	require.Equal(t, recordedStatus{"degraded", "permission_denied", false},
		sink.get("source-pods"), "the reason shows while the sync waits")
	require.Zero(t, readyCalls.Load(),
		"readiness waits for the sources to finish their initial list")

	source.set()
	close(synced)
	tick <- time.Time{} // accepted only after the sync report finished
	cancel()
	<-done

	require.Equal(t, recordedStatus{"running", "", true},
		sink.get("source-pods"))
	require.Positive(t, readyCalls.Load(),
		"the report after the sync sets readiness")
}

func TestSourceHealthRunSurvivesAPanickingReport(t *testing.T) {
	source := &panickingAvailability{}
	health := newSourceHealth(&fakeStatusSink{}, source)
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() { health.run(ctx, tick, nil); close(done) }()

	tick <- time.Time{}
	tick <- time.Time{} // the loop is alive after the first panic
	cancel()
	<-done

	require.GreaterOrEqual(t, source.calls.Load(), int32(1))
}

type panickingAvailability struct{ calls atomic.Int32 }

func (p *panickingAvailability) Unavailable() []kube.SourceStatus {
	p.calls.Add(1)
	panic("source blew up")
}

// staleAvailability is a source that also reports watches that keep
// failing.
type staleAvailability struct {
	fakeAvailability
	stale []kube.SourceStatus
}

func (s *staleAvailability) Stale() []kube.SourceStatus { return s.stale }

func TestSourceHealthShowsAStaleWatchWithoutWithdrawingReadiness(
	t *testing.T,
) {
	sink := &fakeStatusSink{}
	source := &staleAvailability{stale: []kube.SourceStatus{{
		Resource: "pods", Required: true, Reason: kube.ReasonSyncFailed,
	}}}
	var ready []bool
	health := newSourceHealth(sink, source).
		withReadiness(func(ok bool) { ready = append(ready, ok) })

	health.report()

	require.Equal(t, recordedStatus{"degraded", "cache_sync_failed", false},
		sink.get("source-pods"))
	require.Equal(t, []bool{true}, ready,
		"a stale watch is a health signal; readiness is unchanged")

	source.stale = nil
	health.report()

	require.Equal(t, recordedStatus{"running", "", true},
		sink.get("source-pods"), "the watch works again")
}

func TestSourceHealthKeepsTheMoreBasicReasonOverStaleness(t *testing.T) {
	sink := &fakeStatusSink{}
	source := &staleAvailability{stale: []kube.SourceStatus{{
		Resource: "pods", Reason: kube.ReasonSyncFailed,
	}}}
	source.set(kube.SourceStatus{
		Resource: "pods", Required: true, Reason: kube.ReasonAPIUnavailable,
	})
	health := newSourceHealth(sink, source)

	health.report()

	require.Equal(t, "api_unavailable", sink.get("source-pods").reason)
}
