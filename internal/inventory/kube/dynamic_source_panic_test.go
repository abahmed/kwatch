package kube

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordWaits returns a tune function that makes the rebuild wait of a
// dynamic source return at once, and a getter for the waits it was asked
// for.
func recordWaits() (func(*DynamicConfig), func() []time.Duration) {
	var mu sync.Mutex
	var waits []time.Duration
	tune := func(cfg *DynamicConfig) {
		cfg.RestartWait = func(_ context.Context, d time.Duration) bool {
			mu.Lock()
			defer mu.Unlock()
			waits = append(waits, d)
			return true
		}
	}
	return tune, func() []time.Duration {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Duration(nil), waits...)
	}
}

func (f *dynamicFixture) panicWidgets(t *testing.T) {
	t.Helper()
	f.src.mu.Lock()
	w := f.src.running[widgetGVR]
	f.src.mu.Unlock()
	require.NotNil(t, w, "widgets are not watched")
	f.src.panicked(widgetGVR, w, errors.New("boom"), 0)
}

func TestDynamicSourcePanickedWatchIsRebuilt(t *testing.T) {
	tune, waits := recordWaits()
	f := newDynamicFixtureWith(tune, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	require.Eventually(t, f.widgetSynced, testWait, testPoll)

	f.panicWidgets(t)

	assert.False(t, f.watching(widgetGVR))
	assert.Equal(t, KindState{Reason: ReasonSyncFailed}, f.widgetState(t),
		"a dead watch must not report the kind as synced")
	require.Eventually(t, func() bool {
		f.src.mu.Lock()
		defer f.src.mu.Unlock()
		return f.src.retryDue
	}, testWait, testPoll, "the backoff ends and asks for a retry")
	assert.Equal(t, []time.Duration{time.Second}, waits())

	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	assert.True(t, f.watching(widgetGVR))
	require.Eventually(t, f.widgetSynced, testWait, testPoll)
	assert.Empty(t, f.goneObservations(), "a rebuild reports nothing gone")
}

func TestDynamicSourceRepeatedPanicsBackOff(t *testing.T) {
	tune, waits := recordWaits()
	f := newDynamicFixtureWith(tune, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	for range 3 {
		f.panicWidgets(t)
		require.Eventually(t, func() bool {
			f.src.mu.Lock()
			defer f.src.mu.Unlock()
			return f.src.retryDue
		}, testWait, testPoll)
		f.src.reconcile(ctx)
		f.waitSynced(t, ctx)
	}

	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second,
	}, waits())
}
