package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/abahmed/kwatch/internal/inventory"
)

const testPoll = 10 * time.Millisecond

var (
	widgetW = inventory.NewEntityID("example.com", "widget", "ns", "w")
	widgetX = inventory.NewEntityID("example.com", "widget", "ns", "x")
)

// waitGone collects n Gone observations, failing after testWait.
func (f *dynamicFixture) waitGone(
	t *testing.T, n int,
) []inventory.EntityID {
	t.Helper()
	var gone []inventory.EntityID
	timeout := time.After(testWait)
	for len(gone) < n {
		select {
		case o := <-f.observations:
			if o.Kind == inventory.Gone {
				gone = append(gone, o.Entity)
			}
		case <-timeout:
			t.Fatalf("saw %d gone observations, want %d", len(gone), n)
		}
	}
	return gone
}

func (f *dynamicFixture) widgetState(t *testing.T) KindState {
	t.Helper()
	state, known := f.src.KindState("widget")
	require.True(t, known, "widget kind unknown to the dynamic source")
	return state
}

// refuseWidgets refuses the running widget watch, as its reflector's
// error handler would.
func (f *dynamicFixture) refuseWidgets(t *testing.T, err error) {
	t.Helper()
	f.src.mu.Lock()
	w := f.src.running[widgetGVR]
	f.src.mu.Unlock()
	require.NotNil(t, w, "widgets are not watched")
	f.src.refuse(widgetGVR, w, err)
}

func (f *dynamicFixture) widgetSynced() bool {
	state, _ := f.src.KindState("widget")
	return state.Synced
}

func TestDynamicSourcePermissionRefusalKeepsEntities(t *testing.T) {
	f := newDynamicFixtureWith(nil, widgetCRD(), widget("w"), widget("x"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	require.Eventually(t, f.widgetSynced, testWait, testPoll)

	f.refuseWidgets(t, forbidden("widgets"))

	assert.False(t, f.watching(widgetGVR))
	assert.Equal(t, KindState{Reason: ReasonPermissionDenied},
		f.widgetState(t))
	status := f.src.Status()
	assert.Equal(t, 1, status.Unavailable)
	assert.Equal(t, 1, status.Reasons[ReasonPermissionDenied])
	// A triggered discovery waits for the safety timer to retry.
	f.src.reconcile(ctx)
	assert.False(t, f.watching(widgetGVR))
	assert.Empty(t, f.goneObservations(), "refusal reported entities gone")

	// The retried watch reports only what vanished while refused.
	require.NoError(t, f.client.Tracker().Delete(widgetGVR, "ns", "x"))
	f.src.retryRefused()
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	assert.Equal(t, []inventory.EntityID{widgetX}, f.waitGone(t, 1))
	require.Eventually(t, f.widgetSynced, testWait, testPoll)
	assert.Empty(t, f.goneObservations())
}

func TestDynamicSourceRefusalReasons(t *testing.T) {
	notServed := apierrors.NewMethodNotSupported(
		widgetGVR.GroupResource(), "list")
	tt := []struct {
		name   string
		err    error
		reason string
		gone   bool
	}{
		{"forbidden_keeps", forbidden("widgets"),
			ReasonPermissionDenied, false},
		{"unauthorized_keeps", apierrors.NewUnauthorized("expired"),
			ReasonPermissionDenied, false},
		{"method_not_supported_keeps", notServed,
			ReasonAPIUnavailable, false},
		{"not_found_is_gone", apierrors.NewNotFound(
			widgetGVR.GroupResource(), ""), ReasonAPIUnavailable, true},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			f := newDynamicFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer f.stop(cancel)
			f.src.reconcile(ctx)
			f.waitSynced(t, ctx)
			require.Eventually(t, f.widgetSynced, testWait, testPoll)

			f.refuseWidgets(t, tc.err)

			assert.Equal(t, KindState{Reason: tc.reason}, f.widgetState(t))
			if tc.gone {
				assert.Equal(t, []inventory.EntityID{widgetW},
					f.waitGone(t, 1))
				return
			}
			f.src.mu.Lock()
			kept := f.src.refused[widgetGVR].kept
			f.src.mu.Unlock()
			require.NotNil(t, kept)
			<-kept.done
			assert.Empty(t, f.goneObservations())
		})
	}
}

func TestDynamicSourceBudgetDropIsQuiet(t *testing.T) {
	f := newDynamicFixtureWith(func(cfg *DynamicConfig) { cfg.Budget = 3 },
		widgetCRD(), widget("w"), widget("x"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	f.src.cfg.Budget = 2
	f.src.reconcile(ctx)

	assert.False(t, f.watching(widgetGVR))
	assert.Equal(t, KindState{Reason: ReasonWatchBudget}, f.widgetState(t))
	assert.Equal(t, 1, f.src.Status().Skipped)
	f.src.mu.Lock()
	parked := f.src.parked[widgetGVR]
	f.src.mu.Unlock()
	require.NotNil(t, parked)
	<-parked.done
	assert.Empty(t, f.goneObservations(), "budget drop reported gone")

	// Back within budget, the watch takes over the parked entities.
	require.NoError(t, f.client.Tracker().Delete(widgetGVR, "ns", "x"))
	f.src.cfg.Budget = 3
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	assert.Equal(t, []inventory.EntityID{widgetX}, f.waitGone(t, 1))
	require.Eventually(t, f.widgetSynced, testWait, testPoll)
}

func TestDynamicSourceObjectCapMakesKindUnverifiable(t *testing.T) {
	f := newDynamicFixtureWith(
		func(cfg *DynamicConfig) { cfg.MaxObjectsPerKind = 1 },
		widgetCRD(), widget("w"), widget("x"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	assert.Equal(t, KindState{Reason: ReasonObjectCap}, f.widgetState(t))
	s := &Source{cfg: SourceConfig{Dynamic: f.src}}
	assert.False(t, s.Verifiable("widget"))
	assert.False(t, s.Synced("widget"))
	assert.True(t, s.Synced(KindFor("CustomResourceDefinition")))
}

func TestDynamicSourceRecordsMaintenance(t *testing.T) {
	annotated := widget("w")
	annotated.SetAnnotations(map[string]string{"kwatch/maintenance": "on"})
	f := newDynamicFixtureWith(func(cfg *DynamicConfig) {
		cfg.Maintenance = MaintenanceAnnotations{On: "kwatch/maintenance"}
	}, widgetCRD(), annotated)
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)

	f.src.reconcile(ctx)

	timeout := time.After(testWait)
	for {
		select {
		case o := <-f.observations:
			if o.Entity == widgetW && o.Kind == inventory.Observed {
				assert.Equal(t, inventory.Text("on"),
					o.Attributes[AttrMaintenance])
				return
			}
		case <-timeout:
			t.Fatal("widget not observed")
		}
	}
}

func TestDynamicSourceUnknownKindIsUnknown(t *testing.T) {
	src := NewDynamicSource(DynamicConfig{})
	_, known := src.KindState("nothing")
	assert.False(t, known)
}
