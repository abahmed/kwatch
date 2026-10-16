package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	berrors "go.etcd.io/bbolt/errors"

	"github.com/abahmed/kwatch/internal/metrics"
)

// sleepRecorder is the injected sleeper: it records each wait and returns
// at once, so no test waits for a backoff.
type sleepRecorder struct {
	waits []time.Duration
	// onSleep runs at each wait, for example to free a held resource.
	onSleep func(n int)
	err     error
}

func (s *sleepRecorder) sleep(_ context.Context, d time.Duration) error {
	s.waits = append(s.waits, d)
	if s.onSleep != nil {
		s.onSleep(len(s.waits))
	}
	return s.err
}

func (s *sleepRecorder) retry() *rebuildRetry {
	return &rebuildRetry{sleep: s.sleep}
}

func (s *sleepRecorder) total() time.Duration {
	var sum time.Duration
	for _, d := range s.waits {
		sum += d
	}
	return sum
}

// rebuildCounts is the success, retry and failed counters.
func rebuildCounts() [3]int64 {
	r := metrics.DefaultRegistry()
	return [3]int64{
		r.EditionRebuilds[0].Load(), r.EditionRebuilds[1].Load(),
		r.EditionRebuilds[2].Load(),
	}
}

func lockHeld() error {
	return fmt.Errorf("open state: %w", berrors.ErrTimeout)
}

func addrInUse() error {
	return &net.OpError{Op: "listen", Net: "tcp", Err: &netSyscall{}}
}

// netSyscall wraps EADDRINUSE the way the net package reports it.
type netSyscall struct{}

func (*netSyscall) Error() string { return "bind: address already in use" }
func (*netSyscall) Unwrap() error { return syscall.EADDRINUSE }

func TestRebuildRetryBacksOffThenSucceeds(t *testing.T) {
	rec := &sleepRecorder{}
	before := rebuildCounts()
	calls := 0

	err := rec.retry().do(context.Background(), "state store",
		func() error {
			calls++
			if calls < 3 {
				return lockHeld()
			}
			return nil
		})

	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second},
		rec.waits)
	after := rebuildCounts()
	require.Equal(t, before[1]+2, after[1], "one retry count per wait")
}

func TestRebuildRetryGivesUpWithinTheBudget(t *testing.T) {
	rec := &sleepRecorder{}
	calls := 0

	err := rec.retry().do(context.Background(), "health listener",
		func() error { calls++; return addrInUse() })

	require.ErrorIs(t, err, syscall.EADDRINUSE)
	require.ErrorContains(t, err, "health listener: still failing")
	require.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		15 * time.Second,
	}, rec.waits, "doubling waits, the last one cut to the budget")
	require.Equal(t, rebuildRetryBudget, rec.total())
	require.Equal(t, len(rec.waits)+1, calls)
}

func TestRebuildRetryDoesNotRetryPermanentErrors(t *testing.T) {
	rec := &sleepRecorder{}
	calls := 0
	permanent := errors.New("invalid configuration")

	err := rec.retry().do(context.Background(), "bootstrap",
		func() error { calls++; return permanent })

	require.ErrorIs(t, err, permanent)
	require.Equal(t, 1, calls)
	require.Empty(t, rec.waits)
}

func TestRebuildRetryNilRunsOnce(t *testing.T) {
	var retry *rebuildRetry
	calls := 0

	err := retry.do(context.Background(), "state store",
		func() error { calls++; return lockHeld() })

	require.True(t, transientRebuildError(err))
	require.Equal(t, 1, calls, "a first start never retries")
}

func TestRebuildRetryStopsWhenTheContextEnds(t *testing.T) {
	rec := &sleepRecorder{err: context.Canceled}
	calls := 0

	err := rec.retry().do(context.Background(), "state store",
		func() error { calls++; return lockHeld() })

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
	require.Len(t, rec.waits, 1)
}
