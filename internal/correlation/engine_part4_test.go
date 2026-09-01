package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestMarkResolvedIdempotent(t *testing.T) {
	var resolves int
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	ev := event.Event{
		PodName:   "p1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)

	// First MarkResolved should fire the hook
	e.MarkResolved(inc.Key)
	assert.Equal(t, 1, resolves)

	// Second MarkResolved (same key) must NOT fire again
	e.MarkResolved(inc.Key)
	assert.Equal(
		t,
		1,
		resolves,
		"MarkResolved must be idempotent — hook fired twice",
	)
}

func TestMarkResolvedNonexistentKeyNoOp(t *testing.T) {
	var resolves int
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolves++
			}
		},
	})
	e.MarkResolved("nonexistent")
	assert.Equal(t, 0, resolves)
}

func TestResolveHoldDownDelaysResolve(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var resolves int
	e := NewEngine(Config{
		Window:          10 * time.Minute,
		ResolveHoldDown: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolves++
			}
		},
	})
	e.now = mockClock(fakeNow)

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// MarkResolved should NOT fire the hook immediately
	e.MarkResolved(inc.Key)
	assert.Equal(t, 0, resolves)
	live := e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StatePendingResolve, live.State)
		assert.Equal(t, fakeNow.Add(10*time.Minute), live.ResolveAt)
	}
}

func TestCleanupFinalizesPendingResolveWithNotification(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var resolves int
	e := NewEngine(Config{
		Window:          10 * time.Minute,
		ResolveHoldDown: 20 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolves++
			}
		},
	})
	e.now = mockClock(fakeNow)

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// MarkResolved schedules the resolve (ResolveAt = +20m); no notify yet.
	e.MarkResolved(inc.Key)
	assert.Equal(t, 0, resolves)
	if live := e.state[inc.Key]; live != nil {
		assert.Equal(t, model.StatePendingResolve, live.State)
	}

	// Advance past the cleanup window (+10m) but before ResolveAt (+20m):
	// cleanup reaps the incident first and must emit a resolved
	// notification instead of silently dropping it.
	fakeNow = fakeNow.Add(11 * time.Minute)
	e.now = mockClock(fakeNow)
	e.cleanup()

	assert.Equal(
		t,
		1,
		resolves,
		"cleanup must notify a resolved transition for pending-resolve "+
			"incidents",
	)
	assert.Empty(t, e.state, "cleanup must remove the finalized incident")
}

func TestResolveHoldDownRevivesOnRecurrence(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var resolves int
	e := NewEngine(Config{
		Window:          10 * time.Minute,
		ResolveHoldDown: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolves++
			}
		},
	})
	e.now = mockClock(fakeNow)

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Pending resolve
	e.MarkResolved(inc.Key)
	assert.Equal(t, 0, resolves)
	live := e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StatePendingResolve, live.State)
	}

	// Recurrence within cooldown — should revive (skip) and cancel the
	// pending resolve
	_, action = e.Process(ev, "deploy-1", nil)
	assert.Equal(
		t,
		model.ActionSkip,
		action,
		"revive within cooldown must skip, not update",
	)
	live2 := e.state[inc.Key]
	if live2 != nil {
		assert.Equal(
			t,
			model.StateActive,
			live2.State,
			"pending resolve must be revoked",
		)
		assert.True(t, live2.ResolveAt.IsZero(), "ResolveAt must be cleared")
	}
	assert.Equal(t, 0, resolves, "hook must not fire")
}

func TestProcessResolvedIncidentSilentlyRevives(t *testing.T) {
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

	// Immediately resolve — MarkResolved also arms the cooldown.
	e.MarkResolved(key)
	live := e.state[key]
	if live != nil {
		assert.Equal(t, model.StateResolved, live.State)
	}

	// Recurrence within cooldown — suppressed, no notification at all.
	_, action = e.Process(ev, "deploy-1", nil)
	assert.Equal(
		t,
		model.ActionSkip,
		action,
		"recurrence within cooldown must skip",
	)

	// Advance past the cooldown window, then recur again — the resolved
	// incident must silently revive (ActionUpdate, not ActionCreate) to
	// avoid a resolved→CREATE→resolved flip-flop.
	fakeNow = fakeNow.Add(11 * time.Minute)
	e.now = mockClock(fakeNow)
	inc2, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionUpdate, action)
	assert.Equal(t, key, inc2.Key)
	assert.Equal(t, model.StateActive, inc2.State)
}

func TestIncidentKeyMatchesProcess(t *testing.T) {
	tests := []struct {
		name  string
		ev    event.Event
		owner string
		cs    *model.ContainerState
	}{
		{
			name: "CrashLoopBackOff with cs",
			ev: event.Event{
				Namespace: "default",
				Reason:    "CrashLoopBackOff",
			},
			owner: "deploy-1",
			cs:    &model.ContainerState{RestartCount: 3},
		},
		{
			name: "CrashLoopBackOff high frequency",
			ev: event.Event{
				Namespace: "default",
				Reason:    "CrashLoopBackOff",
			},
			owner: "deploy-1",
			cs:    &model.ContainerState{RestartCount: 10},
		},
		{
			name: "normalized reason",
			ev: event.Event{
				Namespace: "default",
				Reason:    "CrashLoopBackOff 42",
			},
			owner: "deploy-1",
			cs:    &model.ContainerState{RestartCount: 1},
		},
		{
			name:  "empty container",
			ev:    event.Event{Namespace: "default", Reason: "OOMKilled"},
			owner: "deploy-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := IncidentKey(tt.ev, tt.owner, tt.cs)

			e := newTestEngine()
			inc, _ := e.Process(tt.ev, tt.owner, tt.cs)
			require.NotNil(t, inc, "Process must produce an incident")
			assert.Equal(t, key1, inc.Key, "IncidentKey must match Process key")
		})
	}
}

func TestCheckLifecycleFinalizesPendingResolve(t *testing.T) {
	var resolved int
	var baselineChanged bool
	e := NewEngine(Config{
		Window:          10 * time.Minute,
		ResolveHoldDown: 1 * time.Millisecond,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolved++
			}
		},
		OnBaselineChange: func(_ map[string]map[string]int64) {
			baselineChanged = true
		},
	})

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	e.MarkResolved(inc.Key)
	live := e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StatePendingResolve, live.State)
	}

	time.Sleep(2 * time.Millisecond)
	e.checkLifecycle()

	assert.Equal(t, 1, resolved)
	assert.True(
		t,
		baselineChanged,
		"OnBaselineChange must fire when pending resolve finalizes",
	)
	live = e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StateResolved, live.State)
	}
}

func TestPerPodBaselineNewPodAlerts(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetBaseline(
		map[string]map[string]int64{string(key): {"pod-1": time.Now().Unix()}},
	)

	// pod-1 is baselined — should skip
	ev1 := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(ev1, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)

	// pod-2 is new — should alert
	ev2 := event.Event{
		Namespace: "default",
		PodName:   "pod-2",
		Reason:    "CrashLoopBackOff",
	}
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestClearBaselineForPodIsPerPod(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetBaseline(
		map[string]map[string]int64{
			string(key): {
				"pod-1": time.Now().Unix(),
				"pod-2": time.Now().Unix(),
			},
		},
	)

	e.ClearBaselineForPod("default", "pod-1")

	// pod-1 un-baselined → create
	ev1 := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(ev1, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// pod-2 still baselined → skip
	ev2 := event.Event{
		Namespace: "default",
		PodName:   "pod-2",
		Reason:    "CrashLoopBackOff",
	}
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}
