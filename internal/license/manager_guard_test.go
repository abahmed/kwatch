package license

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type panickingLeaseStore struct{ memLeaseStore }

func (*panickingLeaseStore) LoadLease() (string, ClockGuard, error) {
	panic("corrupt cached lease")
}

// sleepLog is a Sleep that returns at once for the first n waits and
// then blocks until ctx ends, recording every wait.
type sleepLog struct {
	mu    sync.Mutex
	waits []time.Duration
	free  int
}

func (s *sleepLog) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.waits = append(s.waits, d)
	blocked := len(s.waits) > s.free
	s.mu.Unlock()
	if blocked {
		<-ctx.Done()
	}
	return ctx.Err()
}

func (s *sleepLog) seen() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A cached lease that panics while it is loaded must not crash the
// caller of Start (the leader session): the manager starts without it.
func TestStartSurvivesAPanicWhileLoadingTheCachedLease(t *testing.T) {
	h := newHarness(t)
	h.m.cfg.LeaseStore = &panickingLeaseStore{}
	sleeps := &sleepLog{}
	h.m.cfg.Sleep = sleeps.sleep
	ctx, cancel := context.WithCancel(context.Background())

	h.m.Start(ctx)

	if st := h.m.Status(); st.State == "" {
		t.Fatalf("status after a panic = %+v", st)
	}
	cancel()
	h.m.Wait()
}

// A panic in the first, synchronous sync and in a later cycle is logged
// and retried after panicRetryDelay; the loop keeps running and the
// license activates once the cause is gone.
func TestRenewalLoopSurvivesPanicsAndRecovers(t *testing.T) {
	h := newHarness(t)
	var calls atomic.Int64
	h.m.cfg.NodeCount = func(context.Context) int {
		if calls.Add(1) <= 2 {
			panic("unexpected server response")
		}
		return 3
	}
	h.m.cfg.InitialSyncTimeout = time.Second
	sleeps := &sleepLog{free: 2}
	h.m.cfg.Sleep = sleeps.sleep
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.m.Start(ctx) // the synchronous first sync panics

	waitFor(t, "the license to activate after the panics", func() bool {
		return h.m.Status().State == StateActive
	})
	waits := sleeps.seen()
	if len(waits) < 2 || waits[0] != panicRetryDelay ||
		waits[1] != panicRetryDelay {
		t.Fatalf("waits after panics = %v, want %v twice first",
			waits, panicRetryDelay)
	}
	cancel()
	h.m.Wait()
}
