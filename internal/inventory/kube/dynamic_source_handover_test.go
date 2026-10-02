package kube

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/metrics"
)

// An error from a watch that was already replaced must not refuse the
// watch that replaced it.
func TestDynamicSourceIgnoresErrorsOfAReplacedWatch(t *testing.T) {
	f := newDynamicFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	require.Eventually(t, f.widgetSynced, testWait, testPoll)

	stale := &dynamicWatch{kind: "widget"}
	f.src.refuse(widgetGVR, stale, forbidden("widgets"))

	assert.True(t, f.watching(widgetGVR), "the running watch stays")
	assert.True(t, f.widgetSynced())
}

// A watch that took over from a previous one is not complete until the
// objects that disappeared in between were reported gone.
func TestDynamicSourceKindPendingDuringTakeOver(t *testing.T) {
	d := NewDynamicSource(DynamicConfig{})
	w := &dynamicWatch{
		kind: "widget", hasSynced: func() bool { return true },
		admission:  newAdmission(widgetGVR, 10, nil),
		handedOver: make(chan struct{}),
	}
	d.running[widgetGVR] = w

	state, known := d.KindState("widget")
	require.True(t, known)
	assert.Equal(t, KindState{Reason: ReasonSyncPending}, state)

	close(w.handedOver)
	state, _ = d.KindState("widget")
	assert.Equal(t, KindState{Synced: true}, state)
}

// The dynamic source feeds the watcher metrics: a started watch is a
// sync attempt, a refusal makes an optional API unavailable, and a
// refusal before the first sync is a sync failure.
func TestDynamicSourceCountsWatcherMetrics(t *testing.T) {
	registry := metrics.DefaultRegistry()
	syncs := registry.WatcherSyncs.Load()
	failures := registry.WatcherSyncFailures.Load()
	unavailable := registry.OptionalAPIUnavailable.Load()
	f := newDynamicFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	require.Eventually(t, f.widgetSynced, testWait, testPoll)
	assert.Greater(t, registry.WatcherSyncs.Load(), syncs)

	f.refuseWidgets(t, forbidden("widgets"))
	assert.Equal(t, unavailable+1, registry.OptionalAPIUnavailable.Load())
	assert.Equal(t, failures, registry.WatcherSyncFailures.Load(),
		"the widgets had synced")

	done := make(chan struct{})
	close(done)
	unsynced := &dynamicWatch{kind: "widget",
		hasSynced: func() bool { return false }, cancel: func() {},
		done: done, admission: newAdmission(widgetGVR, 10, f.src.budget)}
	f.src.mu.Lock()
	f.src.running[widgetGVR] = unsynced
	f.src.mu.Unlock()
	f.src.refuse(widgetGVR, unsynced, forbidden("widgets"))
	assert.Equal(t, failures+1, registry.WatcherSyncFailures.Load())
}
