package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestBaselineSuppressesForFullTTL(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newTestEngine()
	e.now = mockClock(fakeNow)
	e.config.BaselineTTL = 24 * time.Hour

	incidentKey := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	// entry created 1 hour ago — well within the 24h TTL
	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"pod-1": fakeNow.Add(-1 * time.Hour).Unix()},
		},
	)

	ev := event.Event{
		PodName: "pod-1", Namespace: "default", Reason: "CrashLoopBackOff",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestBaselineExpiredPrunes(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newTestEngine()
	e.now = mockClock(fakeNow)
	e.config.BaselineTTL = 24 * time.Hour

	incidentKey := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	// entry created 25 hours ago — past the 24h TTL
	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"pod-1": fakeNow.Add(-25 * time.Hour).Unix()},
		},
	)

	ev := event.Event{
		PodName: "pod-1", Namespace: "default", Reason: "CrashLoopBackOff",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// entry should be pruned from baseline
	e.mu.Lock()
	_, stillSeen := e.baseline[string(incidentKey)]
	e.mu.Unlock()
	assert.False(t, stillSeen, "expired baseline entry should be pruned")
}

func TestRemovePodClearsSeen(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})

	// First, create an incident (not baselined)
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	incidentKey := inc.Key

	// Now baseline the incident key
	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"pod-1": time.Now().Unix()},
		},
	)

	// RemovePod clears the baseline for the removed pod
	e.RemovePod("default", "pod-1")

	// A new event for a different pod with the same key — incident is still
	// active (RemovePod does NOT resolve), so the update is silent
	ev2 := event.Event{
		PodName:   "pod-2",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestOwnerLevelBaselineFallsBackToEmptyPod(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			"ns:ns/web:ServiceNoEndpoints:": {"": time.Now().Unix()},
		},
	})
	// Live owner-level signal carries PodName (service name) — must still be
	// recognized as baselined even though the seed used the empty pod key.
	ev := event.Event{
		Resource:  "service",
		PodName:   "web",
		Namespace: "ns",
		Reason:    "ServiceNoEndpoints",
	}
	inc, action := e.Process(ev, "ns/web", nil)
	assert.Nil(t, inc)
	assert.Equal(t, model.ActionSkip, action)
}

func TestOwnerLevelBaselineNoEmptyPod(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			"ns:ns/web:ServiceNoEndpoints:": {"web": time.Now().Unix()},
		},
	})
	ev := event.Event{
		Resource:  "service",
		PodName:   "web",
		Namespace: "ns",
		Reason:    "ServiceNoEndpoints",
	}
	inc, action := e.Process(ev, "ns/web", nil)
	assert.Nil(t, inc)
	assert.Equal(t, model.ActionSkip, action)
}

func TestStsOwnedPodsGroupByStsName(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{
			SeverityByOwnerKind: map[string]string{"StatefulSet": "high"},
		},
	})

	ev1 := event.Event{
		PodName:   "db-0",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
		OwnerKind: "StatefulSet",
	}
	ev2 := event.Event{
		PodName:   "db-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
		OwnerKind: "StatefulSet",
	}

	inc1, action1 := e.Process(ev1, "my-sts", nil)
	inc2, action2 := e.Process(ev2, "my-sts", nil)

	assert.Equal(t, model.ActionCreate, action1)
	assert.Equal(t, model.ActionSkip, action2)
	assert.Equal(t, inc1.Key, inc2.Key)
	assert.Equal(t, "my-sts", inc1.Name)
	assert.Equal(t, model.SeverityHigh, inc1.Severity)
	// After the second call, the live incident has both pods. Use inc2 (clone
	// of the second call).
	assert.True(t, inc2.Resources["db-0"])
	assert.True(t, inc2.Resources["db-1"])
	assert.Equal(t, 2, inc2.Count)
}

func TestSnapshot(t *testing.T) {
	e := newTestEngine()

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	e.Process(ev, "deploy-1", nil)

	snap := e.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 incident in snapshot, got %d", len(snap))
	}
	v := snap[0]
	if v.Key != "default:deploy-1:CrashLoopBackOff:" {
		t.Errorf("unexpected key: %s", v.Key)
	}
	if v.Reason != "CrashLoopBackOff" {
		t.Errorf("unexpected reason: %s", v.Reason)
	}
	if v.Namespace != "default" {
		t.Errorf("unexpected namespace: %s", v.Namespace)
	}
	if v.Count != 1 {
		t.Errorf("unexpected count: %d", v.Count)
	}
	if v.State != model.StateActive {
		t.Errorf("unexpected state: %v", v.State)
	}
}

func TestSnapshotEmpty(t *testing.T) {
	e := newTestEngine()
	snap := e.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected empty snapshot, got %d", len(snap))
	}
}

func TestRenotifyConfig(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		RenotifyIntervalBySeverity: map[string]time.Duration{
			"default": 1 * time.Minute,
		},
		RenotifyMaxPerIncident: 3,
	})
	if v := e.config.RenotifyIntervalBySeverity["default"]; v != 1*time.Minute {
		t.Errorf("unexpected renotify interval: %v", v)
	}
	if e.config.RenotifyMaxPerIncident != 3 {
		t.Errorf("unexpected renotify max: %d", e.config.RenotifyMaxPerIncident)
	}
}

func TestRevivedIncidentResetsRenotifyBudget(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		RenotifyIntervalBySeverity: map[string]time.Duration{
			"default": 1 * time.Minute,
		},
		RenotifyMaxPerIncident: 3,
	})
	e.now = mockClock(fakeNow)

	var updates int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if action == model.ActionUpdate {
			updates++
		}
	}

	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	require.Zero(t, inc.RenotifyCount)

	// Exhaust the renotify budget: 3 renotifies at 61s intervals.
	for i := 0; i < 3; i++ {
		fakeNow = fakeNow.Add(61 * time.Second)
		e.now = mockClock(fakeNow)
		e.checkLifecycle()
		require.Equal(
			t,
			i+1,
			e.state[inc.Key].RenotifyCount,
			"renotify %d must fire",
			i+1,
		)
	}
	require.Equal(t, 3, updates)

	// A 4th cycle past the interval is capped: no more renotifies.
	fakeNow = fakeNow.Add(61 * time.Second)
	e.now = mockClock(fakeNow)
	e.checkLifecycle()
	require.Equal(
		t,
		3,
		e.state[inc.Key].RenotifyCount,
		"renotify budget must cap at maxPer",
	)
	require.Equal(t, 3, updates, "renotify must not exceed maxPer")

	// Resolve, wait out the cooldown, then revive.
	e.MarkResolved(inc.Key)
	require.Equal(t, model.StateResolved, e.state[inc.Key].State)

	fakeNow = fakeNow.Add(11 * time.Minute)
	e.now = mockClock(fakeNow)
	revived, action := e.Process(ev, "deploy-1", nil)
	require.Equal(
		t,
		model.ActionUpdate,
		action,
		"revival must be a silent update, not a create",
	)
	require.Equal(t, model.StateActive, revived.State)
	assert.Zero(
		t,
		revived.RenotifyCount,
		"revival must reset the renotify budget",
	)

	// The revived incident gets a fresh renotify budget.
	e.now = mockClock(fakeNow.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(
		t,
		1,
		e.state[inc.Key].RenotifyCount,
		"revived incident must renotify again",
	)
	// 3 renotifies, the revival update Process announced itself, and the
	// fresh renotify.
	require.Equal(t, 5, updates)
}

// BUG-1: escalation tests.

func escTestEngine() *Engine {
	return NewEngine(Config{
		Window:            10 * time.Minute,
		EscalationEnabled: true,
		EscalationTiers:   []int{3, 10, 50},
	})
}

func TestBarePodIncidentUsesPodName(t *testing.T) {
	e := newTestEngine()

	ev := event.Event{
		PodName:   "bare-pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(
		t,
		"bare-pod-1",
		inc.Name,
		"bare pod incident must use the pod name",
	)
	assert.Equal(t, "pod", inc.Resource)

	// Owner-level incidents keep the owner name
	ev2 := event.Event{
		PodName:   "rs-pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	inc2, _ := e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, "deploy-1", inc2.Name)
}

func TestEscalationFirstCrossingIsHigh(t *testing.T) {
	e := escTestEngine()
	// Use OOMKilled to avoid CrashLoopHighFrequency rename when RestartCount >
	// 5
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	// within cooldown, cross tier 3:
	inc2, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 4})
	assert.Equal(t, model.ActionUpdate, action)
	assert.Equal(t, model.SeverityHigh, inc2.Severity)
	assert.Contains(t, inc2.Hint, "crossed 3")
	assert.Equal(t, inc.Key, inc2.Key)
}
