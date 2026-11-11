package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A panic in the license session's loop is logged and the loop starts
// again; it never ends the process. The session here has no
// dependencies, so its loop panics every time it runs.
func TestLicenseWatchSurvivesAPanickingLoop(t *testing.T) {
	out := captureKlog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var restarts atomic.Int64
	l := &licenseSession{
		restartWait: func(context.Context, time.Duration) bool {
			if restarts.Add(1) == 3 {
				cancel()
				return false
			}
			return true
		},
	}

	l.watch(ctx)
	l.wg.Wait()

	require.Equal(t, int64(3), restarts.Load())
	require.Contains(t, out.String(), "restarting after failure")
}

// A loop that panics before it observed the license is not ready: each
// restart waits longer than the last, instead of once a second forever.
func TestLicenseLoopThatPanicsAtOnceBacksOff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits []time.Duration
	l := &licenseSession{
		restartWait: func(_ context.Context, d time.Duration) bool {
			waits = append(waits, d)
			if len(waits) == 4 {
				cancel()
				return false
			}
			return true
		},
	}

	l.watch(ctx)
	l.wg.Wait()

	require.Len(t, waits, 4)
	for i := 1; i < len(waits); i++ {
		require.Greater(t, waits[i], waits[i-1], "%v", waits)
	}
}
