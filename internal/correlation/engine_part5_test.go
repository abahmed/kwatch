package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestRemovePodReleasesBaseline(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetBaseline(
		map[string]map[string]int64{string(key): {"pod-1": time.Now().Unix()}},
	)

	// RemovePod should release the baseline slot for pod-1
	e.RemovePod("default", "pod-1")

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestRemovePodReleasesBaselineScopedToNamespace(t *testing.T) {
	e := newTestEngine()

	keyA := BuildKey("ns-a", "dep-a", "CrashLoopBackOff", "")
	keyB := BuildKey("ns-b", "dep-b", "CrashLoopBackOff", "")
	e.SetBaseline(map[string]map[string]int64{
		string(keyA): {"web": time.Now().Unix()},
		string(keyB): {"web": time.Now().Unix()},
	})

	// Removing pod "web" from ns-a must not release the baseline slot for
	// the identically-named pod in ns-b.
	e.RemovePod("ns-a", "web")

	assert.NotContains(
		t,
		e.baseline[string(keyA)],
		"web",
		"baseline for ns-a must be released",
	)
	if pods, ok := e.baseline[string(keyB)]; ok {
		assert.Contains(t, pods, "web", "baseline for ns-b must survive")
	} else {
		t.Fatal("ns-b baseline key missing")
	}

	// ns-b pod still baselined → skip
	ev := event.Event{
		Namespace: "ns-b",
		PodName:   "web",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(ev, "dep-b", nil)
	assert.Equal(t, model.ActionSkip, action)

	// ns-a pod un-baselined → create
	ev2 := event.Event{
		Namespace: "ns-a",
		PodName:   "web",
		Reason:    "CrashLoopBackOff",
	}
	_, action = e.Process(ev2, "dep-a", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestResolvedIncidentSilentlyRevives(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	key := inc.Key

	// Resolve (arms cooldown)
	e.MarkResolved(key)
	live := e.state[key]
	if live != nil {
		assert.Equal(t, model.StateResolved, live.State)
	}

	// Advance past the cooldown window so the revival path is exercised.
	fakeNow = fakeNow.Add(11 * time.Minute)
	e.now = mockClock(fakeNow)

	// First recurrence → ActionUpdate (silent revival, not re-create)
	ev2 := event.Event{
		Namespace: "default",
		PodName:   "pod-2",
		Reason:    "CrashLoopBackOff",
	}
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionUpdate, action)

	// Second recurrence → ActionSkip (same sig, no change)
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestPendingReviveSkips(t *testing.T) {
	var resolved int
	e := NewEngine(Config{
		Window:          10 * time.Minute,
		ResolveHoldDown: 60 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolved++
			}
		},
	})

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Mark pending resolve
	e.MarkResolved(inc.Key)
	live := e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StatePendingResolve, live.State)
	}
	assert.Equal(t, 0, resolved)

	// Revive → skip (edge-triggered, same sig), state back to active
	_, action = e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
	live = e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StateActive, live.State)
		assert.True(t, live.ResolveAt.IsZero())
	}
	assert.Equal(t, 0, resolved, "no ActionResolved should be emitted")
}

func TestRemovePodEvictsLastContainerIndex(t *testing.T) {
	e := newTestEngine()

	cs := &model.ContainerState{
		RestartCount: 3,
		Reason:       "CrashLoopBackOff",
		Status:       "waiting",
	}
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	e.Process(ev, "deploy-1", cs)

	key := "default/pod-1/."
	assert.Contains(t, e.lastContainerIndex, key)
	assert.NotNil(t, e.lastContainerIndex[key])
	assert.Equal(t, int32(3), e.lastContainerIndex[key].RestartCount)

	before := len(e.lastContainerIndex)
	e.RemovePod("default", "pod-1")

	assert.NotContains(t, e.lastContainerIndex, key)
	assert.Equal(t, before-1, len(e.lastContainerIndex))
	assert.Nil(t, e.GetLastContainerState("default", "pod-1", "."))
}

func TestLastContainerStateKeyedByContainer(t *testing.T) {
	e := newTestEngine()

	// Two containers in the same pod with different restart counts must
	// not clobber each other's tracked state.
	evApp := event.Event{
		PodName:       "pod-1",
		Namespace:     "default",
		ContainerName: "app",
		Reason:        "CrashLoopBackOff",
	}
	evSidecar := event.Event{
		PodName:       "pod-1",
		Namespace:     "default",
		ContainerName: "sidecar",
		Reason:        "CrashLoopBackOff",
	}
	e.Process(
		evApp,
		"deploy-1",
		&model.ContainerState{
			RestartCount: 9,
			Reason:       "Error",
			Status:       "terminated",
		},
	)
	e.Process(
		evSidecar,
		"deploy-1",
		&model.ContainerState{
			RestartCount: 1,
			Reason:       "Error",
			Status:       "terminated",
		},
	)

	app := e.GetLastContainerState("default", "pod-1", "app")
	if assert.NotNil(t, app) {
		assert.Equal(
			t,
			int32(9),
			app.RestartCount,
			"app container state must not be overwritten by sidecar",
		)
	}
	sidecar := e.GetLastContainerState("default", "pod-1", "sidecar")
	if assert.NotNil(t, sidecar) {
		assert.Equal(t, int32(1), sidecar.RestartCount)
	}
}

// Node baseline, cooldown, and suppression tests.

func TestNodeEventSkipsBaseline(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)
	e.config.BaselineTTL = 24 * time.Hour

	// Seed a baseline entry for the node condition (simulating buildSeenSet)
	incidentKey := BuildKey("", "node-1", "NodeNotReady", "")
	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"node-1": fakeNow.Unix()},
		},
	)

	// Node event with same key — should NOT be baselined
	ev := event.Event{
		Resource: "node",
		PodName:  "node-1",
		NodeName: "node-1",
		Reason:   "NodeNotReady",
	}
	inc, action := e.Process(ev, "node-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.Equal(t, "node", inc.Resource)

	// activeNodeIncidents should be populated
	assert.True(t, e.activeNodeIncidents["node-1"])
}

func TestNodeBaselineDoesNotBlockPodSuppression(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.now = mockClock(fakeNow)
	e.config.BaselineTTL = 24 * time.Hour

	// Pre-populate activeNodeIncidents (simulating buildSeenSet)
	e.activeNodeIncidents["node-1"] = true

	// Pod event on that node — should be suppressed
	podEv := event.Event{
		PodName:   "p1",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestCleanupCooldownSuppressesRecreate(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)

	// Create incident
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Advance past Window so cleanup fires
	fakeNow = fakeNow.Add(11 * time.Minute)
	e.now = mockClock(fakeNow)

	// Store the key for later
	key := BuildKey("ns", "dep", "CrashLoopBackOff", "")

	// Run cleanup — should resolve and add cooldown
	e.cleanup()

	// Cooldown should exist
	e.mu.Lock()
	expiry, hasCooldown := e.cleanupCooldown[key]
	e.mu.Unlock()
	assert.True(t, hasCooldown)
	assert.True(t, expiry.After(fakeNow))

	// Same event re-arrives — should be suppressed by cooldown
	_, action = e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestMarkResolvedSetsCooldown(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)

	// Create incident
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)

	// MarkResolved (no hold-down) must add a cooldown, same as
	// cleanup()/checkLifecycle/ResolveByResource do.
	e.MarkResolved(inc.Key)

	e.mu.Lock()
	expiry, hasCooldown := e.cleanupCooldown[inc.Key]
	e.mu.Unlock()
	assert.True(t, hasCooldown, "MarkResolved must set cleanupCooldown")
	assert.True(t, expiry.After(fakeNow))

	// Same event re-arrives — suppressed by the cooldown, no flip-flop update.
	_, action = e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionSkip, action)
}
