package kube

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

// stepClock is a clock a test moves by hand.
type stepClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// syncedWidgets starts the widget watch, waits for it to sync and
// returns its error handler.
func syncedWidgets(
	t *testing.T, f *dynamicFixture, ctx context.Context,
) func(error) {
	t.Helper()
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	require.Eventually(t, f.widgetSynced, testWait, testPoll)
	f.src.mu.Lock()
	w := f.src.running[widgetGVR]
	f.src.mu.Unlock()
	handler := f.src.watchError(widgetGVR, w)
	return func(err error) { handler(ctx, nil, err) }
}

func TestDynamicSourceSyncedWatchRetriesThroughNotFound(t *testing.T) {
	clock := &stepClock{now: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)}
	f := newDynamicFixtureWith(func(cfg *DynamicConfig) {
		cfg.Now = clock.Now
	}, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	refuse := syncedWidgets(t, f, ctx)
	gone := apierrors.NewNotFound(widgetGVR.GroupResource(), "")

	refuse(gone)
	clock.advance(30 * time.Second)
	refuse(gone)

	assert.True(t, f.watching(widgetGVR), "a brief refusal ended the watch")
	assert.Empty(t, f.goneObservations(), "a brief refusal retired entities")

	// The reflector keeps failing past the grace period.
	for i := 0; i < 2; i++ {
		clock.advance(refusalGrace / 2)
		refuse(gone)
	}

	assert.False(t, f.watching(widgetGVR))
	assert.Equal(t, []inventory.EntityID{widgetW}, f.waitGone(t, 1))
}

func TestDynamicSourceRefusalGraceRestartsAfterRecovery(t *testing.T) {
	clock := &stepClock{now: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)}
	f := newDynamicFixtureWith(func(cfg *DynamicConfig) {
		cfg.Now = clock.Now
	}, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	refuse := syncedWidgets(t, f, ctx)

	refuse(forbidden("widgets"))
	clock.advance(refusalGrace / 2)
	refuse(forbidden("widgets"))
	clock.advance(refusalGrace/2 - time.Second)
	refuse(forbidden("widgets"))
	// The watch served again for a while, so the next refusal is new.
	clock.advance(2 * refusalGap)
	refuse(forbidden("widgets"))

	assert.True(t, f.watching(widgetGVR))
}

func TestDynamicSourceUnsyncedWatchIsRefusedAtOnce(t *testing.T) {
	f := newDynamicFixtureWith(nil, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.src.mu.Lock()
	w := f.src.running[widgetGVR]
	w.hasSynced = func() bool { return false }
	f.src.mu.Unlock()

	f.src.watchError(widgetGVR, w)(ctx, nil, forbidden("widgets"))

	assert.False(t, f.watching(widgetGVR))
	assert.Equal(t, KindState{Reason: ReasonPermissionDenied},
		f.widgetState(t))
}

func TestDynamicSourceDiscoveryTriggerRetriesRefusedTypes(t *testing.T) {
	f := newDynamicFixtureWith(nil, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	f.refuseWidgets(t, forbidden("widgets"))
	done := make(chan struct{})
	go func() { f.src.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	f.src.requestDiscovery()

	require.Eventually(t, func() bool {
		return f.watching(widgetGVR)
	}, testWait, testPoll, "a discovery trigger did not retry the type")
}

func TestDynamicSourceRunSurvivesPanickingPass(t *testing.T) {
	var once sync.Once
	passes := make(chan struct{}, 4)
	f := newDynamicFixtureWith(func(cfg *DynamicConfig) {
		cfg.Reconciled = func(DynamicStatus) {
			passes <- struct{}{}
			once.Do(func() { panic("boom") })
		}
	}, widgetCRD())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.src.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	<-passes

	f.src.requestDiscovery()

	select {
	case <-passes:
	case <-time.After(testWait):
		t.Fatal("no pass after a panicking pass")
	}
	assert.True(t, f.src.Status().Ran, "the source stayed usable")
}

func TestDynamicSourceRetirePanicIsContained(t *testing.T) {
	var armed atomic.Bool
	submitted := make(chan struct{}, 1)
	f := newDynamicFixtureWith(func(cfg *DynamicConfig) {
		cfg.Submit = func(context.Context, ...inventory.Observation) {
			if armed.Load() {
				submitted <- struct{}{}
				panic("boom")
			}
		}
	}, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	armed.Store(true)

	f.refuseWidgets(t, apierrors.NewNotFound(widgetGVR.GroupResource(), ""))

	select {
	case <-submitted:
	case <-time.After(testWait):
		t.Fatal("the retire never reported entities gone")
	}
	f.stop(cancel)
}

// failingWidgets is a widget fixture on a hand-moved clock whose answer
// count the test moves.
func failingWidgets(
	t *testing.T, objects ...runtime.Object,
) (*dynamicFixture, *stepClock, *atomic.Uint64) {
	t.Helper()
	clock := &stepClock{now: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)}
	answers := &atomic.Uint64{}
	f := newDynamicFixtureWith(func(cfg *DynamicConfig) {
		cfg.Now = clock.Now
		cfg.Answered = func(group, resource string) uint64 {
			if group == widgetGVR.Group && resource == widgetGVR.Resource {
				return answers.Load()
			}
			return 0
		}
	}, objects...)
	return f, clock, answers
}

// failWidgets reports err to the running widget watch's error handler.
func (f *dynamicFixture) failWidgets(
	t *testing.T, ctx context.Context, err error,
) {
	t.Helper()
	f.src.mu.Lock()
	w := f.src.running[widgetGVR]
	f.src.mu.Unlock()
	require.NotNil(t, w, "widgets are not watched")
	f.src.watchError(widgetGVR, w)(ctx, &cache.Reflector{}, err)
}

func serverError() error {
	return apierrors.NewInternalError(assert.AnError)
}

func widgetsNamed(status DynamicStatus, reason string) bool {
	for _, k := range status.Kinds {
		if k.Resource == resourceName(widgetGVR) && k.Reason == reason {
			return true
		}
	}
	return false
}

func TestDynamicSourceUnsyncedWatchFailingReportsSyncFailed(t *testing.T) {
	f, clock, _ := failingWidgets(t, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	syncedWidgets(t, f, ctx)
	f.src.mu.Lock()
	f.src.running[widgetGVR].hasSynced = func() bool { return false }
	f.src.mu.Unlock()

	f.failWidgets(t, ctx, serverError())
	assert.Equal(t, ReasonSyncPending, f.widgetState(t).Reason)
	clock.advance(dynamicStaleAfter)
	f.failWidgets(t, ctx, serverError())

	assert.Equal(t, KindState{Reason: ReasonSyncFailed}, f.widgetState(t))
	status := f.src.Status()
	assert.True(t, widgetsNamed(status, ReasonSyncFailed), status.Kinds)
	assert.Zero(t, status.Unavailable)
	assert.True(t, f.watching(widgetGVR), "a server error ended the watch")
}

func TestDynamicSourceSyncedWatchFailingIsStaleUntilAnswered(t *testing.T) {
	f, clock, answers := failingWidgets(t, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	syncedWidgets(t, f, ctx)
	s := &Source{cfg: SourceConfig{Dynamic: f.src}}

	f.failWidgets(t, ctx, serverError())
	clock.advance(dynamicStaleAfter - time.Second)
	require.True(t, s.Fresh("widget"), "stale before the bound")
	clock.advance(time.Second)

	assert.False(t, s.Fresh("widget"))
	assert.Equal(t, KindState{Synced: true, Stale: true}, f.widgetState(t))
	assert.True(t, s.Verifiable("widget"), "stale is not unverifiable")
	assert.True(t, widgetsNamed(f.src.Status(), ReasonSyncFailed))

	answers.Add(1)

	assert.True(t, s.Fresh("widget"), "an answered relist ends the run")
	assert.False(t, widgetsNamed(f.src.Status(), ReasonSyncFailed))
}

func TestDynamicSourceRefusalStartsNoFailureRun(t *testing.T) {
	f, clock, _ := failingWidgets(t, widgetCRD(), widget("w"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	syncedWidgets(t, f, ctx)

	f.failWidgets(t, ctx, forbidden("widgets"))
	clock.advance(2 * dynamicStaleAfter)

	assert.Equal(t, KindState{Synced: true}, f.widgetState(t))
	assert.True(t, f.watching(widgetGVR))
}
