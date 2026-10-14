package safego

import (
	"context"
	"errors"
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

func skipWaits(t *testing.T) *[]time.Duration {
	var waits []time.Duration
	old := Wait
	Wait = func(_ context.Context, d time.Duration) bool {
		waits = append(waits, d)
		return true
	}
	t.Cleanup(func() { Wait = old })
	return &waits
}

func TestKeepRestartsAfterPanic(t *testing.T) {
	skipWaits(t)
	runs := 0
	Keep(context.Background(), "demo", func(ready func()) error {
		runs++
		if runs == 1 {
			panic("boom")
		}
		ready()
		return nil
	}, nil)

	require.Equal(t, 2, runs)
}

func TestKeepReportsSickThenRecovers(t *testing.T) {
	waits := skipWaits(t)
	var states []bool
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
	}, func(v bool) { states = append(states, v) })

	require.Equal(t, []bool{true, false}, states)
	require.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
	}, (*waits)[:4])
}

func TestKeepStopsWithContext(t *testing.T) {
	skipWaits(t)
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	Keep(ctx, "demo", func(func()) error {
		runs++
		cancel()
		return errors.New("fail")
	}, nil)
	require.Equal(t, 1, runs)
}

func TestBackoffCaps(t *testing.T) {
	require.Equal(t, time.Second, Backoff(1))
	require.Equal(t, time.Minute, Backoff(20))
}
