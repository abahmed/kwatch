package safego

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunTurnsPanicIntoError(t *testing.T) {
	err := Run("demo", func() error { panic("boom") })

	require.ErrorContains(t, err, "demo panicked: boom")
}

func TestRunPassesThroughResult(t *testing.T) {
	require.NoError(t, Run("demo", func() error { return nil }))
}

func TestGoSurvivesPanic(t *testing.T) {
	done := make(chan struct{})
	Go("demo", func() {
		defer close(done)
		panic("boom")
	})
	<-done
}

// skipWaits returns Options whose Wait returns at once and records the
// waits.
func skipWaits() (Options, *[]time.Duration) {
	var waits []time.Duration
	return Options{Wait: func(_ context.Context, d time.Duration) bool {
		waits = append(waits, d)
		return true
	}}, &waits
}

func TestKeepRestartsAfterPanic(t *testing.T) {
	opts, _ := skipWaits()
	runs := 0
	Keep(context.Background(), "demo", func(ready func()) error {
		runs++
		if runs == 1 {
			panic("boom")
		}
		ready()
		return nil
	}, opts)

	require.Equal(t, 2, runs)
}

func TestKeepReportsSickThenRecovers(t *testing.T) {
	opts, waits := skipWaits()
	var states []bool
	opts.Sick = func(v bool) { states = append(states, v) }
	runs := 0
	Keep(context.Background(), "demo", func(ready func()) error {
		runs++
		if runs <= 4 {
			panic("boom")
		}
		ready()
		if runs == 5 {
			return errors.New("fails after working")
		}
		return nil
	}, opts)

	require.Equal(t, []bool{true, false}, states)
	require.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
	}, (*waits)[:4])
}

func TestKeepStopsWithContext(t *testing.T) {
	opts, _ := skipWaits()
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	Keep(ctx, "demo", func(func()) error {
		runs++
		cancel()
		return errors.New("fail")
	}, opts)
	require.Equal(t, 1, runs)
}

func TestBackoffCaps(t *testing.T) {
	require.Equal(t, time.Second, Backoff(1))
	require.Equal(t, time.Minute, Backoff(20))
}

func TestKeepWithoutOptionsSleepsOnARealTimer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		Keep(ctx, "demo", func(func()) error {
			runs.Add(1)
			return errors.New("fail")
		}, Options{})
	}()
	require.Eventually(t, func() bool { return runs.Load() >= 1 },
		time.Second, time.Millisecond)
	cancel() // ends the one-second backoff sleep at once
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Keep did not stop with its context")
	}
}
