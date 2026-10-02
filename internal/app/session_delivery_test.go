package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// lastSendStopper stands in for the delivery manager. Its Stop records
// how it was called and, when it may send, creates a thread the way a
// final delivery would.
type lastSendStopper struct {
	mu      sync.Mutex
	threads *fakeThreads
	calls   int
	ctxDone bool
}

func (s *lastSendStopper) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.ctxDone = ctx.Err() != nil
	if !s.ctxDone {
		s.threads.mu.Lock()
		s.threads.threads = sampleThreads
		s.threads.mu.Unlock()
	}
	return ctx.Err()
}

func TestFinishActiveSessionSavesThreadsCreatedByFinalDrain(t *testing.T) {
	state := openTestStore(t)
	disk := diskState{store: state}
	source := &fakeThreads{}
	stopper := &lastSendStopper{threads: source}
	guard := &storeGuard{}
	guard.abandon() // keep the store open so the test can read it back

	finishActiveSession(context.Background(), activeShutdown{
		supervisor: newComponentSupervisor(time.Now),
		delivery:   stopper, threads: newThreadSaver(source, disk),
		session: &recordingEnder{}, state: state, guard: guard,
	})

	require.Equal(t, 1, stopper.calls)
	require.False(t, stopper.ctxDone, "a graceful drain may still send")
	restored := &fakeThreads{}
	newThreadSaver(restored, disk).Restore()
	require.Equal(t, sampleThreads, restored.threads,
		"thread IDs from the final drain were not saved")
}

func TestFinishActiveSessionFencesDeliveryAfterLeadershipLoss(t *testing.T) {
	state := openTestStore(t)
	source := &fakeThreads{}
	stopper := &lastSendStopper{threads: source}

	finishActiveSession(context.Background(), activeShutdown{
		supervisor: newComponentSupervisor(time.Now),
		delivery:   stopper,
		threads:    newThreadSaver(source, diskState{store: state}),
		session:    &recordingEnder{}, state: state,
		leadershipLost: true,
	})

	require.Equal(t, 1, stopper.calls)
	require.True(t, stopper.ctxDone,
		"delivery must not send after leadership is lost")
	require.Nil(t, source.threads)
}

func TestLeadershipLostOnlyWhenLeaderContextEndsFirst(t *testing.T) {
	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()
	leaderCtx, loseLease := context.WithCancel(appCtx)
	deps := &serverDeps{ctx: appCtx}

	require.False(t, leadershipLost(leaderCtx, deps), "still leading")
	loseLease()
	require.True(t, leadershipLost(leaderCtx, deps))
	stopApp()
	require.False(t, leadershipLost(leaderCtx, deps),
		"a normal shutdown keeps the Lease until shutdown finishes")
}
