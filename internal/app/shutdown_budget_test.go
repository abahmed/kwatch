package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery"
)

func TestShutdownBudgetNestsInsideTerminationGrace(t *testing.T) {
	require.Less(t, totalShutdownBudget, terminationGracePeriod)
	require.Greater(t, componentShutdownTimeout, watcherStopTimeout)
	require.Greater(t, supervisorShutdownTimeout, componentShutdownTimeout)
	require.GreaterOrEqual(t, activeSessionTimeout,
		supervisorShutdownTimeout+deliveryDrainTimeout+
			threadFinalTimeout+sessionEndTimeout)
	require.Greater(t, backgroundShutdownTimeout, activeSessionTimeout)
}

type recordingEnder struct {
	reason   string
	deadline bool
	release  chan struct{}
}

func (r *recordingEnder) EndSession(ctx context.Context, reason string) {
	_, r.deadline = ctx.Deadline()
	r.reason = reason
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
		}
	}
}

func TestEndSessionIsBoundedAndIgnoresParentCancellation(t *testing.T) {
	ender := &recordingEnder{}
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	require.True(t, endSession(parent, ender, "graceful_shutdown",
		time.Second))

	require.Equal(t, "graceful_shutdown", ender.reason)
	require.True(t, ender.deadline, "session end must carry a deadline")
}

func TestEndSessionReportsTimeout(t *testing.T) {
	// The ender blocks until its ctx ends, as a hung write would.
	ender := &recordingEnder{release: make(chan struct{})}

	require.False(t, endSession(context.Background(), ender, "x",
		time.Millisecond))
}

// When a step of the session shutdown does not finish, the store stays
// open: closing it under a running writer is unsafe.
func TestFinishActiveSessionKeepsStoreOpenAfterTimeout(t *testing.T) {
	state := openTestStore(t)
	disk := diskState{store: state}
	source := &blockingThreads{
		entered: make(chan struct{}, 1), release: make(chan struct{}),
	}
	defer close(source.release)
	threads := newThreadSaver(source, disk)
	threads.flushTimeout = time.Millisecond

	finishActiveSession(context.Background(), activeShutdown{
		supervisor: newComponentSupervisor(time.Now),
		threads:    threads, session: &recordingEnder{}, state: state,
	})

	require.NoError(t, disk.put("probe", true), "store closed early")
}

// A storage writer abandoned at its deadline may still be inside a slow
// write; the store must stay open for it.
func TestFinishActiveSessionKeepsStoreOpenForAbandonedWriter(t *testing.T) {
	state := openTestStore(t)
	disk := diskState{store: state}
	threads := newThreadSaver(&fakeThreads{threads: sampleThreads}, disk)
	guard := &storeGuard{}
	guard.abandon()

	finishActiveSession(context.Background(), activeShutdown{
		supervisor: newComponentSupervisor(time.Now),
		threads:    threads, session: &recordingEnder{}, state: state,
		guard: guard,
	})

	require.NoError(t, disk.put("probe", true), "store closed under writer")
}

func TestFinishActiveSessionClosesStoreAfterCleanStop(t *testing.T) {
	state := openTestStore(t)
	disk := diskState{store: state}
	ender := &recordingEnder{}
	threads := newThreadSaver(&fakeThreads{threads: sampleThreads}, disk)

	finishActiveSession(context.Background(), activeShutdown{
		supervisor: newComponentSupervisor(time.Now),
		threads:    threads, session: ender, state: state, failed: true,
	})

	require.Equal(t, "internal_failure", ender.reason)
	require.Error(t, disk.put("probe", true), "store left open")
	require.ErrorIs(t, threads.Save(context.Background()),
		errThreadSaverClosed)
}

func TestRunDeliveryMarksReadyOnlyAfterStart(t *testing.T) {
	deps := componentDeps()
	deps.readiness = newReadinessCoordinator(nil)
	deps.readiness.begin(1, true)
	manager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}})
	require.NoError(t, manager.Stop(context.Background()))
	deps.deliveryManager = manager

	err := runDelivery(context.Background(), deps)

	require.Error(t, err, "a stopped manager cannot start")
	require.False(t, deps.readiness.ready["delivery"])

	deps.deliveryManager = delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, runDelivery(ctx, deps))
	require.True(t, deps.readiness.ready["delivery"])
}

// The election must wait for the whole session shutdown (components, the
// delivery drain, the thread flush and the session end), not just one
// component's stop, or a normal drain is reported as a stuck session.
func TestElectionWaitsForTheWholeSessionShutdown(t *testing.T) {
	require.Equal(t, activeSessionTimeout, activeSessionWait)
	require.Greater(t, activeSessionWait,
		supervisorShutdownTimeout+deliveryDrainTimeout)
}
