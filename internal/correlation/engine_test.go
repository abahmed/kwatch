package correlation

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func mockClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func newTestEngine() *Engine {
	return NewEngine(Config{
		Window: 10 * time.Minute,
	})
}

func TestNewEngine(t *testing.T) {
	e := newTestEngine()
	assert.NotNil(t, e)
	assert.NotNil(t, e.state)
}

func TestProcessCreateNew(t *testing.T) {
	e := newTestEngine()

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	owner := "deploy-1"

	inc, action := e.Process(ev, owner, nil)

	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.Equal(t, "default:deploy-1:CrashLoopBackOff:", string(inc.Key))
	assert.Equal(t, "deploy-1", inc.Name)
	assert.Equal(t, "default", inc.Namespace)
	assert.Equal(t, "CrashLoopBackOff", inc.Reason)
	assert.Equal(t, 1, inc.Count)
	assert.Equal(t, 1, len(inc.Resources))
	assert.True(t, inc.Resources["pod-1"])
	assert.True(t, inc.FirstSeen.Equal(inc.LastSeen))
}

func TestProcessRepeatedEventSkipsSameSig(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	// First event creates
	inc1, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Second event with identical sig → skip (edge-triggered), but Count
	// still updates
	ev.PodName = "pod-2"
	inc2, action2 := e.Process(ev, "deploy-1", nil)

	assert.Equal(t, model.ActionSkip, action2)
	assert.Equal(t, inc1.Key, inc2.Key)
	assert.Equal(t, 2, inc2.Count)
	assert.True(t, inc2.Resources["pod-1"])
	assert.True(t, inc2.Resources["pod-2"])
}

func TestProcessSkipSameSigSkipsButUpdatesCount(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	inc1, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Second event with same sig → skip (edge-triggered), Count and Resources
	// still update
	ev.PodName = "pod-2"
	inc2, action2 := e.Process(ev, "deploy-1", nil)

	assert.Equal(t, model.ActionSkip, action2)
	assert.Equal(t, inc1.Key, inc2.Key)
	assert.Equal(t, 2, inc2.Count)
	assert.True(t, inc2.Resources["pod-1"])
	assert.True(t, inc2.Resources["pod-2"])
}

func TestProcessDifferentOwnerNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Same namespace+reason but different owner
	_, action2 := e.Process(ev, "deploy-2", nil)
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessDifferentReasonNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Same namespace+owner but different reason
	ev.Reason = "OOMKilled"
	_, action2 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessDifferentNamespaceNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Different namespace
	ev.Namespace = "kube-system"
	_, action2 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessEmptyOwner(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "OOMKilled",
	}

	inc, action := e.Process(ev, "", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, "default::OOMKilled:", string(inc.Key))
}

func TestCleanup(t *testing.T) {
	e := newTestEngine()
	e.config.Window = 1 * time.Millisecond

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	e.Process(ev, "deploy-1", nil)
	assert.Equal(t, 1, len(e.state))

	time.Sleep(2 * time.Millisecond)
	e.cleanup()
	assert.Equal(t, 0, len(e.state))
}

func TestCleanupKeepsRecent(t *testing.T) {
	e := newTestEngine()
	e.config.Window = 1 * time.Hour

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	e.Process(ev, "deploy-1", nil)
	assert.Equal(t, 1, len(e.state))

	e.cleanup()
	assert.Equal(t, 1, len(e.state))
}

func TestRemovePodNoResolve(t *testing.T) {
	e := newTestEngine()

	ev1 := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	ev2 := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "OOMKilled",
	}

	e.Process(ev1, "deploy-1", nil)
	e.Process(ev2, "deploy-1", nil)

	assert.Equal(t, 2, len(e.state))

	// RemovePod must NOT resolve incidents — just remove the pod from resources
	e.RemovePod("default", "pod-1")

	assert.Equal(
		t,
		model.StateActive,
		e.state["default:deploy-1:CrashLoopBackOff:"].State,
		"incident must stay active after RemovePod",
	)
	assert.Equal(
		t,
		model.StateActive,
		e.state["default:deploy-1:OOMKilled:"].State,
		"incident must stay active after RemovePod",
	)
	assert.Equal(
		t,
		0,
		len(e.state["default:deploy-1:CrashLoopBackOff:"].Resources),
	)
	assert.Equal(t, 0, len(e.state["default:deploy-1:OOMKilled:"].Resources))
}

func TestProcessConcurrentSafe(t *testing.T) {
	e := newTestEngine()
	e.config.Window = time.Hour

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := event.Event{
				PodName:   "pod-1",
				Namespace: "default",
				Reason:    "CrashLoopBackOff",
			}
			e.Process(ev, "deploy-1", nil)
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, len(e.state))
	inc := e.state["default:deploy-1:CrashLoopBackOff:"]
	assert.Equal(t, 100, inc.Count)
}

func TestBaselineSuppression(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newTestEngine()
	e.now = mockClock(fakeNow)

	incidentKey := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")

	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"pod-1": fakeNow.Unix()},
		},
	)

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestSetBaselineMergesNotReplaces(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newTestEngine()
	e.now = mockClock(fakeNow)

	key1 := BuildKey("default", "dep-1", "CrashLoopBackOff", "")
	key2 := BuildKey("default", "dep-1", "OOMKilled", "")
	key3 := BuildKey("default", "dep-2", "CrashLoopBackOff", "")

	// First call: seed key1 and key2
	e.SetBaseline(map[string]map[string]int64{
		string(key1): {"pod-a": fakeNow.Unix()},
		string(key2): {"pod-b": fakeNow.Unix()},
	})

	// Second call: same key1 with fresher timestamp, plus new key3
	later := fakeNow.Add(1 * time.Hour)
	e.SetBaseline(map[string]map[string]int64{
		string(key1): {"pod-a": later.Unix()},
		string(key3): {"pod-c": later.Unix()},
	})

	// All keys should be present (key1 and key2 preserved from first call,
	// key3 from second call, key1 timestamp updated)
	e.mu.Lock()
	defer e.mu.Unlock()

	_, ok1 := e.baseline[string(key1)]["pod-a"]
	assert.True(
		t,
		ok1,
		"key1 from first SetBaseline must survive after second SetBaseline",
	)

	_, ok2 := e.baseline[string(key2)]["pod-b"]
	assert.True(
		t,
		ok2,
		"key2 from first SetBaseline must survive after second SetBaseline "+
			"(merge)",
	)

	_, ok3 := e.baseline[string(key3)]["pod-c"]
	assert.True(t, ok3, "key3 from second SetBaseline must be present")

	// Timestamp for key1/pod-a must reflect the later value (was updated, not
	// stuck)
	assert.Equal(t, later.Unix(), e.baseline[string(key1)]["pod-a"],
		"SetBaseline must update timestamp for existing entry")
}

func TestClearSeenUnsuppresses(t *testing.T) {
	e := newTestEngine()

	incidentKey := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")

	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"pod-1": time.Now().Unix()},
		},
	)
	e.ClearBaselineForPod("default", "pod-1")

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

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

// ── BUG-1: escalation ──────────────────────────────────────────────

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

func TestEscalationSecondCrossingIsCritical(t *testing.T) {
	e := NewEngine(Config{
		Window:            10 * time.Minute,
		EscalationEnabled: true,
		EscalationTiers:   []int{1, 3, 5},
	})
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 0})
	e.Process(
		ev,
		"dep",
		&model.ContainerState{RestartCount: 2},
	) // crosses tier 1 → high
	inc, action := e.Process(
		ev,
		"dep",
		&model.ContainerState{RestartCount: 4},
	) // crosses tier 3 → critical
	assert.Equal(t, model.ActionUpdate, action)
	assert.Equal(t, model.SeverityCritical, inc.Severity)
}

func TestEscalationSameTierSkips(t *testing.T) {
	e := escTestEngine()
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 4})
	_, action := e.Process(
		ev,
		"dep",
		&model.ContainerState{RestartCount: 5},
	) // 4→5: no tier, same sig
	assert.Equal(t, model.ActionSkip, action)
}

func TestEscalationDisabledIsNoop(t *testing.T) {
	e := newTestEngine() // escalation off
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	_, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 4})
	assert.Equal(t, model.ActionSkip, action) // no escalation, same sig
}

// ── BUG-2: inhibition ──────────────────────────────────────────────

func TestInhibitionSuppressesPodOnBrokenNode(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	nodeEv := event.Event{
		Resource: "node",
		PodName:  "node-1",
		NodeName: "node-1",
		Reason:   "NodeNotReady",
	}
	e.Process(nodeEv, "node-1", nil)
	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(podEv, "dep", nil)
	assert.Nil(t, inc)
	assert.Equal(t, model.ActionSkip, action)
}

func TestInhibitionFlagOffDoesNotSuppress(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: false,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	_, action := e.Process(
		event.Event{
			PodName:   "p",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"dep",
		nil,
	)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionOtherNodeUnaffected(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-2",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionLiftsOnNodeResolve(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	e.ResolveByResource("node", "node-1")
	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionLiftsOnNodeResolveDuringHoldDown(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		ResolveHoldDown:           5 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	assert.True(t, e.activeNodeIncidents["node-1"])

	// Node recovers: the incident enters PendingResolve (hold-down), but the
	// recovered node must stop suppressing pods immediately, not after the
	// hold-down finalizes.
	e.ResolveByResource("node", "node-1")
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"recovered node must stop suppressing pods during hold-down",
	)

	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionMarkResolvedHoldDownClearsFlag(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		ResolveHoldDown:           5 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	assert.True(t, e.activeNodeIncidents["node-1"])

	// MarkResolved with hold-down enabled must clear the flag the same way
	// the immediate-resolve branch does.
	e.MarkResolved(BuildKey("", "node-1", "NodeNotReady", ""))
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"recovered node must stop suppressing pods during hold-down",
	)
}

func TestInhibitionSuppressedCounter(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"dep",
		nil,
	)
	nodeInc := e.findNodeIncident("node-1")
	if nodeInc != nil {
		assert.Equal(t, 1, nodeInc.SuppressedPods)
		if assert.NotNil(t, nodeInc.SuppressedOwners) {
			assert.Equal(t, 1, nodeInc.SuppressedOwners["dep"])
		}
	}
}

func TestInhibitionOvercommitDoesNotSuppressPods(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	// Synthetic capacity incident — not a real node outage.
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeResourceCritical",
		},
		"node-1",
		nil,
	)
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"overcommit must not populate inhibition map",
	)

	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"pod on over-committed node must not be suppressed",
	)
}

func TestInhibitionOvercommitDoesNotSuppressUnschedulable(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeResourceCritical",
		},
		"node-1",
		nil,
	)

	ev := event.Event{
		PodName:   "p1",
		Namespace: "ns",
		NodeName:  "",
		Reason:    "Unschedulable",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"overcommit on one node must not suppress cluster-wide unschedulable "+
			"pods",
	)
}

func TestInhibitionRecoveredBaselineNodeClearsFlag(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	// Simulate a pre-existing NodeNotReady at startup (baselined, no incident).
	e.SetActiveNodeIncidents([]string{"node-1"})
	assert.True(t, e.activeNodeIncidents["node-1"])

	// Node recovers — no incident exists to resolve, but the flag must clear.
	e.RefreshNodeInhibition("node-1")
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"recovered node must stop suppressing pods",
	)

	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionRefreshKeepsFlagWithOtherIncidents(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	// Two disruptive conditions on the same node.
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeMemoryPressure",
		},
		"node-1",
		nil,
	)

	// One condition resolves — the other still active must keep the flag.
	e.MarkResolved(BuildKey("", "node-1", "NodeNotReady", ""))
	e.RefreshNodeInhibition("node-1")
	assert.True(
		t,
		e.activeNodeIncidents["node-1"],
		"remaining active node incident must keep suppression",
	)

	// Once the last active node incident resolves, the flag clears.
	e.MarkResolved(BuildKey("", "node-1", "NodeMemoryPressure", ""))
	e.RefreshNodeInhibition("node-1")
	assert.False(t, e.activeNodeIncidents["node-1"])
}

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

// ── Node baseline / cooldown / suppression tests ──────────────────

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

func TestCleanupCooldownExpires(t *testing.T) {
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

	// Advance past Window + 1s so cooldown expires
	fakeNow = fakeNow.Add(11*time.Minute + 1*time.Second)
	e.now = mockClock(fakeNow)

	// Cleanup
	e.cleanup()

	// Advance past Window again (cooldown = Window = 10 min)
	fakeNow = fakeNow.Add(11 * time.Minute)
	e.now = mockClock(fakeNow)

	// Same event — should create new incident (cooldown expired)
	inc, action := e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
}

func TestSuppressedOwnersTracked(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})

	// Create node incident and populate inhibition
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)

	// Suppress pods from different owners on the same node
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"deploy-1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "OOMKilled",
		},
		"deploy-1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p3",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"statefulset-1",
		nil,
	)

	// Verify SuppressedOwners on the node incident
	nodeInc := e.findNodeIncident("node-1")
	if assert.NotNil(t, nodeInc) {
		assert.Equal(t, 3, nodeInc.SuppressedPods)
		assert.Equal(t, 2, nodeInc.SuppressedOwners["deploy-1"])
		assert.Equal(t, 1, nodeInc.SuppressedOwners["statefulset-1"])
	}
}

func TestUnschedulableSuppressedDuringNodeIncident(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})

	// Create node incident
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)

	// Unschedulable pod (empty NodeName) — should be suppressed
	ev := event.Event{
		PodName:   "p1",
		Namespace: "ns",
		NodeName:  "",
		Reason:    "Unschedulable",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)

	// Verify SuppressedPods incremented on the node incident
	nodeInc := e.findNodeIncident("node-1")
	if assert.NotNil(t, nodeInc) {
		assert.Equal(t, 1, nodeInc.SuppressedPods)
		assert.Equal(t, 1, nodeInc.SuppressedOwners["deploy-1"])
	}
}

// ── mock listers for isOwnerHealthy tests ─────────────────────

type mockDeployLister struct {
	appsv1lister.DeploymentLister
	getFn func(ns, name string) (*appsv1.Deployment, error)
}

func (m *mockDeployLister) Deployments(
	namespace string,
) appsv1lister.DeploymentNamespaceLister {
	return &mockDeployNsLister{
		getFn: func(name string) (*appsv1.Deployment, error) {
			return m.getFn(namespace, name)
		},
	}
}

type mockDeployNsLister struct {
	appsv1lister.DeploymentNamespaceLister
	getFn func(name string) (*appsv1.Deployment, error)
}

func (m *mockDeployNsLister) Get(name string) (*appsv1.Deployment, error) {
	return m.getFn(name)
}

func (m *mockDeployNsLister) List(
	selector labels.Selector,
) ([]*appsv1.Deployment, error) {
	return nil, nil
}

type mockSSLister struct {
	appsv1lister.StatefulSetLister
	getFn func(ns, name string) (*appsv1.StatefulSet, error)
}

func (m *mockSSLister) StatefulSets(
	namespace string,
) appsv1lister.StatefulSetNamespaceLister {
	return &mockSSNsLister{
		getFn: func(name string) (*appsv1.StatefulSet, error) {
			return m.getFn(namespace, name)
		},
	}
}

type mockSSNsLister struct {
	appsv1lister.StatefulSetNamespaceLister
	getFn func(name string) (*appsv1.StatefulSet, error)
}

func (m *mockSSNsLister) Get(name string) (*appsv1.StatefulSet, error) {
	return m.getFn(name)
}

func (m *mockSSNsLister) List(
	selector labels.Selector,
) ([]*appsv1.StatefulSet, error) {
	return nil, nil
}

type mockDSLister struct {
	appsv1lister.DaemonSetLister
	getFn func(ns, name string) (*appsv1.DaemonSet, error)
}

func (m *mockDSLister) DaemonSets(
	namespace string,
) appsv1lister.DaemonSetNamespaceLister {
	return &mockDSNsLister{getFn: func(name string) (*appsv1.DaemonSet, error) {
		return m.getFn(namespace, name)
	}}
}

type mockDSNsLister struct {
	appsv1lister.DaemonSetNamespaceLister
	getFn func(name string) (*appsv1.DaemonSet, error)
}

func (m *mockDSNsLister) Get(name string) (*appsv1.DaemonSet, error) {
	return m.getFn(name)
}

func (m *mockDSNsLister) List(
	selector labels.Selector,
) ([]*appsv1.DaemonSet, error) {
	return nil, nil
}

func TestSnapshotAllRestoreIncidentsRoundTrip(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	require.NotNil(t, inc)
	require.NotEmpty(t, inc.Key)

	// Snapshot
	snap := e.SnapshotAll()
	require.NotNil(t, snap)
	require.Contains(t, snap, inc.Key)

	snapped := snap[inc.Key]
	assert.Equal(t, inc.Reason, snapped.Reason)
	assert.Equal(t, inc.Count, snapped.Count)
	assert.Equal(t, inc.State, snapped.State)

	// Restore into a fresh engine with matching baseline
	e2 := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			string(inc.Key): {"pod-1": time.Now().Unix()},
		},
	})
	e2.RestoreIncidents(snap)
	assert.Equal(t, 1, e2.ActiveCount())

	// Verify restored incidents are accessible
	e2.mu.Lock()
	restored, exists := e2.state[inc.Key]
	e2.mu.Unlock()
	assert.True(t, exists)
	assert.Equal(t, inc.Reason, restored.Reason)
	assert.Equal(t, inc.Namespace, restored.Namespace)
	assert.False(t, restored.LastSeen.IsZero())
}

func TestSnapshotPersistedRoundTrip(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	require.NotNil(t, inc)

	snap := e.SnapshotPersisted()
	require.NotNil(t, snap)
	require.Len(t, snap, 1)
	assert.Equal(t, inc.Key, snap[0].Key)
	assert.Equal(t, inc.Reason, snap[0].Reason)
	assert.Equal(t, inc.State, snap[0].State)

	e2 := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			string(inc.Key): {"pod-1": time.Now().Unix()},
		},
	})
	restored := make(map[model.IncidentKey]*model.Incident, len(snap))
	for i := range snap {
		restored[snap[i].Key] = snap[i].ToIncident()
	}
	e2.RestoreIncidents(restored)
	assert.Equal(t, 1, e2.ActiveCount())
}

func TestMassFailurePersistenceRoundTrip(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	mfKey := MassFailureKey("configmap/ns/app-cfg")
	created := e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:       mfKey,
			Reason:    "CrashLoopBackOff",
			Namespace: "ns",
			Resource:  "pod",
			Name:      "configmap/ns/app-cfg",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	},
	)
	assert.True(t, created)
	assert.True(t, e.HasMassFailure(mfKey))
	// Duplicate add is a no-op.
	assert.False(t, e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key: mfKey,
		},
	}))

	snap := e.SnapshotPersisted()
	require.NotNil(t, snap)
	keys := make([]model.IncidentKey, 0, len(snap))
	for i := range snap {
		keys = append(keys, snap[i].Key)
	}
	assert.Contains(t, keys, mfKey)

	// Restore into a fresh engine WITHOUT any baseline: mass failures bypass
	// the baseline gate, so they survive restarts.
	e2 := NewEngine(Config{Window: 10 * time.Minute})
	restored := make(map[model.IncidentKey]*model.Incident, len(snap))
	for i := range snap {
		restored[snap[i].Key] = snap[i].ToIncident()
	}
	e2.RestoreIncidents(restored)
	assert.True(t, e2.HasMassFailure(mfKey))

	// Removing it clears the store.
	assert.True(t, e2.RemoveMassFailure(mfKey))
	assert.False(t, e2.HasMassFailure(mfKey))
	assert.False(t, e2.RemoveMassFailure(mfKey))
}

func TestMassFailureKeyHelpers(t *testing.T) {
	assert.True(t, IsMassFailureKey(MassFailureKey("node//n1")))
	assert.False(t, IsMassFailureKey("ns:dep:CrashLoopBackOff:"))

	parts := ParseKey(MassFailureKey("configmap/ns/app-cfg"))
	assert.True(t, parts.IsMassFailure)
	assert.Equal(t, "configmap/ns/app-cfg", parts.MassDependencyKey)

	// Non mass-failure keys parse as before.
	normal := ParseKey("ns:dep:CrashLoopBackOff:")
	assert.False(t, normal.IsMassFailure)
}

func TestMassFailureSetClone(t *testing.T) {
	e := NewEngine(Config{Window: 10 * time.Minute})
	e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:      MassFailureKey("node//n1"),
			Reason:   "NotReady",
			Resource: "node",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	})

	snap := e.MassFailureSet()
	require.Len(t, snap, 1)
	// Mutating the snapshot must not corrupt the store.
	for _, inc := range snap {
		inc.State = model.StateResolved
	}
	assert.True(t, e.HasMassFailure(MassFailureKey("node//n1")))
}

func TestSnapshotAllEmpty(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	snap := e.SnapshotAll()
	// No incidents processed so dirty=false and SnapshotAll returns nil
	assert.Nil(t, snap)
}

func TestActiveIncidentsDoesNotClearDirty(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	_, _ = e.Process(ev, "dep", &model.ContainerState{RestartCount: 1})

	// Multiple consumers within a tick: ActiveIncidents must behave the same
	// for every call AND leave SnapshotAll able to report the incident.
	first := e.ActiveIncidents()
	require.Len(t, first, 1)
	second := e.ActiveIncidents()
	require.Len(t, second, 1)
	assert.Equal(t, first, second)

	snap := e.SnapshotAll()
	require.Len(t, snap, 1)
}

func TestRestoreIncidentsBumpsLastSeen(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{PodName: "pod-1", Namespace: "ns", Reason: "OOMKilled"}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 1})
	require.NotNil(t, inc)
	originalLastSeen := inc.LastSeen

	time.Sleep(time.Millisecond)

	snap := e.SnapshotAll()
	e2 := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			string(inc.Key): {"pod-1": time.Now().Unix()},
		},
	})
	e2.RestoreIncidents(snap)

	e2.mu.Lock()
	restored, exists := e2.state[inc.Key]
	e2.mu.Unlock()
	assert.True(t, exists, "restored incident must exist in state")
	assert.True(
		t,
		restored.LastSeen.After(originalLastSeen),
		"expected restored LastSeen (%v) to be after original (%v)",
		restored.LastSeen,
		originalLastSeen,
	)
}

func TestIsOwnerHealthyDeploymentHealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       3,
					UnavailableReplicas: 0,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}

	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDeploymentUnhealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       2,
					UnavailableReplicas: 1,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDeploymentNotObserved(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  1,
					Replicas:            3,
					ReadyReplicas:       3,
					UnavailableReplicas: 0,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDeploymentNotFound(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return nil, fmt.Errorf("not found")
		},
	})

	// With resources → unhealthy (keep incident open)
	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
		Status: model.Status{
			Resources: map[string]bool{"p": true},
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))

	// Without resources → healthy (safe to resolve)
	inc2 := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc2))
}

func TestIsOwnerHealthyNonPodResource(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-node",
			OwnerKind: "",
			Resource:  "node",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyNilListers(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyStatefulSet(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetStatefulSetLister(&mockSSLister{
		getFn: func(ns, name string) (*appsv1.StatefulSet, error) {
			return &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					ReadyReplicas:      3,
					CurrentRevision:    "rev-2",
					UpdateRevision:     "rev-2",
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-ss",
			OwnerKind: "StatefulSet",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDaemonSet(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDaemonSetLister(&mockDSLister{
		getFn: func(ns, name string) (*appsv1.DaemonSet, error) {
			return &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberUnavailable:      0,
					UpdatedNumberScheduled: 3,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-ds",
			OwnerKind: "DaemonSet",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDaemonSetUnhealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDaemonSetLister(&mockDSLister{
		getFn: func(ns, name string) (*appsv1.DaemonSet, error) {
			return &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberUnavailable:      1,
					UpdatedNumberScheduled: 2,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-ds",
			OwnerKind: "DaemonSet",
			Resource:  "pod",
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))
}

func TestClearBaselineForPodClearsCooldown(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)

	key := BuildKey("ns", "dep", "CrashLoopBackOff", "")

	// Manually add cooldown entry
	e.mu.Lock()
	e.cleanupCooldown[key] = fakeNow.Add(10 * time.Minute)
	e.mu.Unlock()

	// ClearBaselineForPod for the pod's namespace
	e.ClearBaselineForPod("ns", "pod-1")

	// Cooldown should be cleared
	e.mu.Lock()
	_, exists := e.cleanupCooldown[key]
	e.mu.Unlock()
	assert.False(t, exists, "cooldown should be cleared by ClearBaselineForPod")
}

// ── Smart grouping (reason-adaptive) tests ─────────────────────────

func newSmartGroupingEngine() *Engine {
	return NewEngine(Config{
		Window:              10 * time.Minute,
		SmartGroupingWindow: 60 * time.Second,
	})
}

// The first owner to fail in a namespace is announced at once; grouping only
// starts holding incidents back from the second owner, which is the earliest
// point a fan-out can be told apart from an isolated failure.
func TestSmartGroupingBuffersSameReason(t *testing.T) {
	e := newSmartGroupingEngine()
	_, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"the first owner alerts immediately",
	)
	assert.Equal(t, 1, len(e.state))
	_, action = e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep2",
		nil,
	)
	assert.Equal(
		t,
		model.ActionSkip,
		action,
		"the second owner is buffered — a fan-out may be starting",
	)
	assert.Equal(
		t,
		2,
		len(e.state),
		"buffered incidents are still added to state",
	)
	var hooks int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		hooks++
	}
	e.checkLifecycle()
	assert.Equal(t, 0, hooks, "no hooks before window expiry")
}

// A node going away made twelve deployments unready at once. The first owner
// alerts immediately; the rest are held for one window, and if enough owners
// fail the same way they collapse into a single namespace-wide alert that also
// absorbs the first one (closing its individual thread).
func TestNamespaceFanOutCollapsesIntoOneAlert(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newEngine := func(threshold int) *Engine {
		e := NewEngine(
			Config{
				Window:                   10 * time.Minute,
				SmartGroupingWindow:      60 * time.Second,
				NamespaceFanOutThreshold: threshold,
			},
		)
		e.now = mockClock(now)
		return e
	}
	type emitted struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	fail := func(
		e *Engine, ns string, owners ...string,
	) (direct []model.IncidentAction) {
		for _, o := range owners {
			_, a := e.Process(
				event.Event{
					PodName:   o + "-abc",
					Namespace: ns,
					Reason:    "ContainersNotReady",
				},
				o,
				nil,
			)
			if a != model.ActionSkip {
				direct = append(direct, a)
			}
		}
		return direct
	}
	collect := func(e *Engine) []emitted {
		var got []emitted
		e.config.LifecycleHook = func(
			inc *model.Incident, a model.IncidentAction,
		) {
			got = append(got, emitted{inc, a})
		}
		e.now = mockClock(now.Add(61 * time.Second))
		e.checkLifecycle()
		return got
	}
	groupCreates := func(got []emitted) (n int, last *model.Incident) {
		for _, g := range got {
			if IsGroupKey(g.inc.Key) && g.action == model.ActionCreate {
				n++
				last = g.inc
			}
		}
		return
	}

	// Six owners: the first alerts now; at window end, one alert for all six.
	e := newEngine(3)
	direct := fail(
		e,
		"dev",
		"readify",
		"api",
		"tracking",
		"accounts",
		"tdesk",
		"fleet",
	)
	assert.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate},
		direct,
		"only the first owner alerts before the window",
	)
	got := collect(e)
	n, group := groupCreates(got)
	require.Equal(t, 1, n, "a namespace-wide fan-out is one event, not six")
	assert.Equal(
		t,
		6,
		group.Count,
		"the collapsed alert must account for every owner, the first included",
	)
	var noted, falselyResolved bool
	for _, g := range got {
		if g.inc.Key == "dev:readify:ContainersNotReady:" {
			if g.action == model.ActionUpdate {
				noted = true
			}
			if g.action == model.ActionResolved {
				falselyResolved = true
			}
		}
	}
	assert.True(
		t,
		noted,
		"the first owner's thread gets a note pointing at the namespace-wide "+
			"alert",
	)
	assert.False(
		t,
		falselyResolved,
		"a pod that is still down must never be marked resolved",
	)

	// The first owner recovers later: its own thread gets the resolve.
	// (collect installed a hook bound to its own slice; rebind to ours.)
	got = nil
	e.config.LifecycleHook = func(
		inc *model.Incident, a model.IncidentAction,
	) {
		got = append(got, emitted{inc, a})
	}
	e.MarkResolved("dev:readify:ContainersNotReady:")
	var ownResolve bool
	for _, g := range got {
		if g.inc.Key == "dev:readify:ContainersNotReady:" &&
			g.action == model.ActionResolved {
			ownResolve = true
		}
	}
	assert.True(
		t,
		ownResolve,
		"the first owner resolves on its own thread when it actually recovers",
	)

	// Below the threshold: both owners are plain incidents.
	e2 := newEngine(3)
	direct2 := fail(e2, "dev", "readify", "api")
	got2 := collect(e2)
	assert.Len(t, direct2, 1, "first owner immediate")
	require.Len(t, got2, 1, "second owner released as itself at window end")
	assert.False(t, IsGroupKey(got2[0].inc.Key))
	assert.Equal(t, model.ActionCreate, got2[0].action)

	// Different namespaces do not merge with each other.
	e3 := newEngine(3)
	fail(e3, "dev", "a1", "a2", "a3")
	fail(e3, "prod", "b1", "b2", "b3")
	n3, _ := groupCreates(collect(e3))
	assert.Equal(t, 2, n3, "one collapsed alert per namespace")

	// The feature can be turned off: one notification per owner, none grouped.
	e4 := newEngine(0)
	direct4 := fail(
		e4,
		"dev",
		"readify",
		"api",
		"tracking",
		"accounts",
	)
	got4 := collect(e4)
	assert.Len(t, direct4, 1)
	assert.Len(
		t,
		got4,
		3,
		"the three buffered owners are each released as themselves",
	)
	for _, g := range got4 {
		assert.False(t, IsGroupKey(g.inc.Key))
	}
}

// Node-scoped group keys have three parts too ("reason|node|<name>"). Three
// different nodes failing the same way are three incidents, not one
// "namespace" called "node".
// A per-owner group that has been announced and is then absorbed by a
// namespace fan-out must have its thread closed, not left open forever.
//
// Through Process alone an owner-scoped buffer only ever holds one incident
// (the key is owner-scoped too), so an announced per-owner group is not
// reachable that way today. The fold still has to be correct: the flush state
// is seeded directly here, as any future path that produces one would leave it.
func TestNamespaceFanOutClosesTheGroupItAbsorbs(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(
		Config{
			Window:                   10 * time.Minute,
			SmartGroupingWindow:      60 * time.Second,
			NamespaceFanOutThreshold: 3,
		},
	)
	e.now = mockClock(now)
	type seen struct {
		key    model.IncidentKey
		action model.IncidentAction
	}
	var log []seen
	e.config.LifecycleHook = func(
		inc *model.Incident, a model.IncidentAction,
	) {
		log = append(log, seen{inc.Key, a})
	}

	// dep1's group was announced earlier and its thread is open.
	e.groupFlushStates["ContainersNotReady|ns|dep1"] = &groupFlushState{
		notified:       true,
		lastNotifiedAt: now.Add(-time.Hour),
		firstSeen:      now.Add(-time.Hour),
	}

	for _, dep := range []string{"dep1", "dep2", "dep3"} {
		e.Process(
			event.Event{
				PodName:   dep + "-c",
				Namespace: "ns",
				Reason:    "ContainersNotReady",
			},
			dep,
			nil,
		)
	}
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	var resolvedPerOwner, createdFanOut bool
	for _, s := range log {
		if s.key == "__group__:ContainersNotReady|ns|dep1" &&
			s.action == model.ActionResolved {
			resolvedPerOwner = true
		}
		if s.key == "__group__:ContainersNotReady|ns|*" &&
			s.action == model.ActionCreate {
			createdFanOut = true
		}
	}
	assert.True(
		t,
		resolvedPerOwner,
		"the absorbed per-owner group must be resolved, not orphaned",
	)
	assert.True(t, createdFanOut, "the namespace-wide alert takes over")
	_, still := e.groupFlushStates["ContainersNotReady|ns|dep1"]
	assert.False(t, still, "the absorbed group's flush state is forgotten")
}

func TestNamespaceFanOutDoesNotMergeNodeScopedGroups(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(
		Config{
			Window:                   10 * time.Minute,
			SmartGroupingWindow:      60 * time.Second,
			NamespaceFanOutThreshold: 3,
		},
	)
	e.now = mockClock(now)
	for _, node := range []string{"node-a", "node-b", "node-c"} {
		e.Process(
			event.Event{
				Resource: "node",
				PodName:  node,
				NodeName: node,
				Reason:   "DiskPressure",
			},
			node,
			nil,
		)
	}
	var got []*model.Incident
	e.config.LifecycleHook = func(
		inc *model.Incident, _ model.IncidentAction,
	) {
		got = append(got, inc)
	}
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Len(
		t,
		got,
		3,
		"one alert per node; a node is not an owner in a namespace",
	)
	for _, inc := range got {
		assert.NotContains(
			t,
			string(inc.Key),
			"|*",
			"no fan-out key may be synthesised for node groups",
		)
	}
}

// A symptom suppressed under a mass failure is tracked, not dropped: it counts
// toward the mass failure, resolves silently, and — if it is still broken when
// the mass failure clears — is announced then instead of vanishing.
func TestMassFailureSuppressionReleasesSurvivorsWhenItClears(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	deadNode := MassFailureKey("node//ip-10-0-81-7")
	e := NewEngine(Config{
		Window: 10 * time.Minute,
		DependenciesOf: func(inc *model.Incident) []string {
			if inc.NodeName != "" {
				return []string{"node//" + inc.NodeName}
			}
			return nil
		},
	})
	e.now = mockClock(now)
	e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:    deadNode,
			Reason: "ContainersNotReady",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	})

	type seen struct {
		name   string
		action model.IncidentAction
	}
	var announced []seen
	e.config.LifecycleHook = func(inc *model.Incident, a model.IncidentAction) {
		if a != model.ActionSkip {
			announced = append(announced, seen{inc.Name, a})
		}
	}

	ev := func(dep string) event.Event {
		return event.Event{
			PodName:   dep + "-abc",
			Namespace: "ns",
			Reason:    "ContainersNotReady",
			NodeName:  "ip-10-0-81-7",
		}
	}
	for _, dep := range []string{"dep1", "dep2", "dep3"} {
		inc, action := e.Process(ev(dep), dep, nil)
		assert.Equal(t, model.ActionSkip, action)
		require.NotNil(t, inc, "a suppressed incident is still recorded")
		assert.Equal(t, deadNode, inc.SuppressedBy)
	}
	assert.Equal(
		t,
		3,
		e.ActiveCount(),
		"suppressed incidents count toward the mass failure",
	)
	assert.Empty(
		t,
		announced,
		"nothing under a mass failure is announced on its own",
	)

	// One member recovers while suppressed: no notification for something never
	// announced.
	e.MarkResolved(BuildKey("ns", "dep1", "ContainersNotReady", ""))
	assert.Empty(t, announced, "a suppressed incident resolves silently")

	// The mass failure clears; the two survivors are released and announced
	// by the engine itself — no caller has to remember to do it.
	released := e.ReleaseSuppressed(deadNode)
	require.Equal(t, 2, released, "only still-active members are released")
	require.Len(t, announced, 2)
	for _, a := range announced {
		assert.Equal(t, model.ActionCreate, a.action)
		assert.NotEqual(
			t,
			"dep1",
			a.name,
			"the resolved member is not re-announced",
		)
	}
	// From here on they behave like any other incident: they were announced
	// on release, so their recovery is announced too.
	e.MarkResolved(BuildKey("ns", "dep2", "ContainersNotReady", ""))
	assert.Equal(
		t,
		seen{"dep2", model.ActionResolved},
		announced[len(announced)-1],
		"a released incident's recovery is visible",
	)
}

func TestMassFailureSuppressesItsMembers(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	deadNode := "node//ip-10-0-81-7"

	e := NewEngine(Config{
		Window: 10 * time.Minute,
		DependenciesOf: func(inc *model.Incident) []string {
			if inc.NodeName != "" {
				return []string{"node//" + inc.NodeName}
			}
			return nil
		},
	})
	e.now = mockClock(now)

	// Without a tracked mass failure, each workload alerts on its own.
	for _, dep := range []string{"dep1", "dep2", "dep3"} {
		_, action := e.Process(event.Event{
			PodName: dep + "-abc", Namespace: "ns",
			Reason: "ContainersNotReady", NodeName: "ip-10-0-81-7",
		}, dep, nil)
		assert.NotEqual(
			t,
			model.ActionSkip,
			action,
			"%s should alert while nothing explains it",
			dep,
		)
	}

	// The detector now attributes the failures to one dead node.
	e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:    MassFailureKey(deadNode),
			Reason: "ContainersNotReady",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	},
	)

	// Further workloads on that node are symptoms of an alert already sent.
	for _, dep := range []string{"dep4", "dep5", "dep6"} {
		_, action := e.Process(event.Event{
			PodName: dep + "-abc", Namespace: "ns",
			Reason: "ContainersNotReady", NodeName: "ip-10-0-81-7",
		}, dep, nil)
		assert.Equal(
			t,
			model.ActionSkip,
			action,
			"%s is covered by the mass-failure alert",
			dep,
		)
	}

	// A workload on a healthy node is unrelated and must still alert.
	_, action := e.Process(event.Event{
		PodName: "dep7-abc", Namespace: "ns",
		Reason: "ContainersNotReady", NodeName: "ip-10-0-99-9",
	}, "dep7", nil)
	assert.NotEqual(
		t,
		model.ActionSkip,
		action,
		"a different node is not covered",
	)

	// The node's own incident is the root cause, never a symptom of itself.
	_, action = e.Process(event.Event{
		Resource: "node", PodName: "ip-10-0-81-7", Namespace: "",
		Reason: "NodeNotReady", NodeName: "ip-10-0-81-7",
	}, "ip-10-0-81-7", nil)
	assert.NotEqual(
		t,
		model.ActionSkip,
		action,
		"node incidents must never be suppressed",
	)
}

// A lone owner is not a group. It is announced at once, under its own key, and
// the grouping window emits nothing for it later.
func TestSmartGroupingSingleMemberEmitsIncidentNotGroup(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	inc, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"no waiting for a grouping window",
	)
	assert.Equal(t, model.IncidentKey("ns:dep1:CrashLoopBackOff:"), inc.Key)
	assert.False(t, IsGroupKey(inc.Key))

	var emitted int
	e.config.LifecycleHook = func(
		*model.Incident, model.IncidentAction,
	) {
		emitted++
	}
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	assert.Equal(
		t,
		0,
		emitted,
		"nothing left to emit for an already-announced lone owner",
	)

	// Two members that share a real dimension still become a group.
	sigLog := "connection refused:5432"
	e2 := newSmartGroupingEngine()
	e2.now = mockClock(now)
	e2.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e2.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	var grouped bool
	e2.config.LifecycleHook = func(
		inc *model.Incident, _ model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			grouped = true
		}
	}
	e2.now = mockClock(now.Add(61 * time.Second))
	e2.checkLifecycle()
	assert.True(t, grouped, "two members must still be announced as a group")
}

func TestSmartGroupingFlushAfterWindow(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var groupInc *model.Incident
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupInc = inc
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	require.NotNil(t, groupInc, "group summary must be emitted")
	assert.Equal(t, "CrashLoopBackOff", groupInc.Reason)
	assert.Equal(t, 2, groupInc.Count)
	assert.Contains(t, groupInc.Hint, "dep1")
	assert.Contains(t, groupInc.Hint, "dep2")
}

func TestSmartGroupingDifferentReasonsSeparate(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "OOMKilled"},
		"dep1",
		nil,
	)

	var groups int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groups++
		}
	}
	e.checkLifecycle()
	assert.Equal(t, 0, groups)
}

func TestSmartGroupingResolvedNotIncluded(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	// Three members share a signature, so removing one still leaves a real
	// group of two rather than a lone incident.
	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p3",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep3",
		nil,
	)

	e.MarkResolved("ns:dep1:CrashLoopBackOff:")

	var groupCount int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupCount++
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	require.Equal(t, 1, groupCount)
}

func TestBuildGroupSummary(t *testing.T) {
	e := newSmartGroupingEngine()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.now = mockClock(now)
	entries := []groupEntry{
		{
			namespace: "ns",
			owner:     "dep1",
			reason:    "CrashLoopBackOff",
			podName:   "p1",
		},
		{
			namespace: "ns",
			owner:     "dep2",
			reason:    "CrashLoopBackOff",
			podName:   "p2",
		},
		{
			namespace: "ns",
			owner:     "dep1",
			reason:    "CrashLoopBackOff",
			podName:   "p3",
		},
	}
	summary := e.buildGroupSummary(entries)
	// The reason, the count and the age are rendered as their own fields; the
	// summary names what is affected and nothing else.
	assert.NotContains(
		t,
		summary,
		"CrashLoopBackOff",
		"reason is shown separately",
	)
	assert.NotContains(t, summary, "total", "count is shown separately")
	assert.Contains(t, summary, "2 workloads in ns")
	assert.Contains(
		t,
		summary,
		"dep1 ×2",
		"an owner with several failing pods says so",
	)
	assert.Contains(t, summary, "dep2")
	_ = now
}

func TestBuildGroupSummaryEmpty(t *testing.T) {
	e := newSmartGroupingEngine()
	assert.Equal(t, "", e.buildGroupSummary(nil))
	assert.Equal(t, "", e.buildGroupSummary([]groupEntry{}))
}

func TestSmartGroupingWindowConfigZeroDisabled(t *testing.T) {
	e := NewEngine(Config{
		Window:              10 * time.Minute,
		SmartGroupingWindow: 0,
	})
	_, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"window=0 should disable buffering",
	)
}

func TestSmartGroupingPendingGroupCleanedAfterFlush(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	e.mu.Lock()
	pg, exists := e.groupBuffers["CrashLoopBackOff|ns|dep1"]
	e.mu.Unlock()
	assert.False(t, exists, "pending group must be deleted after flush")
	require.Nil(t, pg)
}

func TestSmartGroupingIncidentHasNotifiedSig(t *testing.T) {
	e := newSmartGroupingEngine()
	// First owner: announced immediately, so its signature is the real one.
	inc, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(t, model.ActionCreate, action)
	require.NotNil(t, inc)
	assert.NotZero(t, inc.NotifiedSig, "NotifiedSig must be set")
	assert.NotZero(t, inc.LastNotifiedAt, "LastNotifiedAt must be set")
	// Second owner: buffered, and the signature is set to hold it back.
	inc2, action := e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep2",
		nil,
	)
	assert.Equal(t, model.ActionSkip, action)
	require.NotNil(t, inc2)
	assert.NotZero(
		t,
		inc2.NotifiedSig,
		"a buffered incident carries a signature so it is not re-buffered",
	)
}

func TestSmartGroupingReFlushUpdateNotCreate(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	// Two owners sharing a log signature form one genuine group. A buffer
	// holding a single member is emitted as that member, not as a group.
	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var groupInc *model.Incident
	var groupAction model.IncidentAction
	var groupActions int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupInc = inc
			groupAction = action
			groupActions++
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, 1, groupActions)
	require.Equal(t, model.ActionCreate, groupAction)
	key := groupInc.Key

	// Re-arm the buffer with more events on the same group.
	e.Process(
		event.Event{
			PodName:   "p3",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p4",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	// Flush again past the renotify cooldown: same key, UPDATE not CREATE.
	e.now = mockClock(now.Add(7 * time.Minute))
	e.checkLifecycle()

	require.Equal(t, 2, groupActions)
	assert.Equal(
		t,
		key,
		groupInc.Key,
		"re-flush must keep the stable group key",
	)
	assert.Equal(
		t,
		model.ActionUpdate,
		groupAction,
		"re-flush must emit an update, not a create",
	)
}

func TestSmartGroupingReFlushCooldownSuppresses(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var groupCalls int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupCalls++
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	assert.Equal(t, 1, groupCalls)

	// Re-arm the buffer and flush within the cooldown: no re-notification.
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(122 * time.Second))
	e.checkLifecycle()
	assert.Equal(
		t,
		1,
		groupCalls,
		"re-flush within the cooldown must not re-notify",
	)
}

func TestSmartGroupingReGroupAfterCooldownSkip(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var groupCalls int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupCalls++
		}
	}

	// First flush emits CREATE and resets the member's NotifiedSig.
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, 1, groupCalls)

	// Re-arm with the same member and flush within the renotify cooldown:
	// suppressed, but the member must remain re-groupable afterward.
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(122 * time.Second))
	e.checkLifecycle()
	require.Equal(
		t,
		1,
		groupCalls,
		"re-flush within the cooldown must not re-notify",
	)

	// The suppressed flush must reset NotifiedSig so the member can re-enter
	// the buffer; once the cooldown lapses the recurring flush must emit an
	// UPDATE rather than staying silent forever.
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(400 * time.Second))
	e.checkLifecycle()
	assert.Equal(
		t,
		2,
		groupCalls,
		"group must resume UPDATE notifications after the cooldown lapses",
	)
}

func TestSmartGroupingFoldRekeysMember(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var actions []model.IncidentAction
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			actions = append(actions, action)
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// Member dep1 crosses the high-frequency threshold → folded. The incident
	// is migrated to the folded key (not dropped), so the group keeps tracking
	// it under the new key instead of being released early.
	cs := &model.ContainerState{RestartCount: 6}
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		cs,
	)

	_, ok := e.state["ns:dep1:CrashLoopHighFrequency:"]
	require.True(t, ok, "folded incident must migrate to the folded key")
	_, ok = e.state["ns:dep1:CrashLoopBackOff:"]
	require.False(t, ok, "old key must be gone")
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions,
		"fold must not resolve or re-notify the group")

	// Resolving only dep2 must not resolve the group — dep1 is still crashing
	// under its folded key.
	e.MarkResolved("ns:dep2:CrashLoopBackOff:")
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// Resolving dep1 (folded key) resolves the whole group.
	e.MarkResolved("ns:dep1:CrashLoopHighFrequency:")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate, model.ActionResolved},
		actions,
	)
}

func TestSmartGroupingFoldRekeysAllMembers(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var actions []model.IncidentAction
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			actions = append(actions, action)
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// Both members fold → both migrate to folded keys. The group still tracks
	// them (the loops are ongoing), so it must NOT resolve.
	cs := &model.ContainerState{RestartCount: 6}
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		cs,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		cs,
	)
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions,
		"folding must not resolve the group synchronously")

	e.now = mockClock(now.Add(90 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions,
		"folding must not resolve the group on the next tick either")
}

func TestResolveByResourceReleasesGroupMember(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var actions []model.IncidentAction
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			actions = append(actions, action)
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// Resolving one member via ResolveByResource must not emit a group
	// resolve until every member has resolved.
	e.ResolveByResource("pod", "dep2")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate},
		actions,
		"group not fully resolved yet",
	)

	e.ResolveByResource("pod", "dep1")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate, model.ActionResolved},
		actions,
	)
}

func TestSmartGroupingNewOccurrenceCreatesAgain(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var actions []model.IncidentAction
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			actions = append(actions, action)
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// All members resolve → batch group resolve resets the flush state.
	e.MarkResolved("ns:dep1:CrashLoopBackOff:")
	e.MarkResolved("ns:dep2:CrashLoopBackOff:")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate, model.ActionResolved},
		actions,
	)

	// A new occurrence of the same group (after the member cooldown) must
	// CREATE again rather than updating the previously-resolved key.
	e.now = mockClock(now.Add(11*time.Minute + 2*time.Second))
	e.Process(
		event.Event{
			PodName:   "p3",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p4",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(12*time.Minute + 5*time.Second))
	e.checkLifecycle()

	require.Len(t, actions, 3)
	assert.Equal(t, model.ActionCreate, actions[2])
}

// ── Reason-adaptive scope tests ────────────────────────────────────

func TestSmartGroupingOwnerScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "OOMKilled"},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "OOMKilled"},
		"dep2",
		nil,
	)
	e.mu.Lock()
	_, has1 := e.groupBuffers["OOMKilled|ns|dep1"]
	_, has2 := e.groupBuffers["OOMKilled|ns|dep2"]
	e.mu.Unlock()
	assert.False(t, has1, "the first owner is announced, not buffered")
	assert.True(
		t,
		has2,
		"the second owner is buffered under its own owner-scoped key",
	)
}

func TestSmartGroupingNodeScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(
		event.Event{
			PodName:  "node-1",
			Resource: "node",
			NodeName: "node-1",
			Reason:   "DiskPressure",
		},
		"node-1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:  "node-2",
			Resource: "node",
			NodeName: "node-2",
			Reason:   "DiskPressure",
		},
		"node-2",
		nil,
	)

	e.mu.Lock()
	_, has1 := e.groupBuffers["DiskPressure|node|node-1"]
	_, has2 := e.groupBuffers["DiskPressure|node|node-2"]
	e.mu.Unlock()
	assert.True(t, has1, "node-1 group must exist")
	assert.True(t, has2, "node-2 group must exist")
}

func TestSmartGroupingSignatureScope(t *testing.T) {
	e := newSmartGroupingEngine()
	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns1",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns2",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	gk := "CrashLoopBackOff|sig|Postgres unreachable — check the DB " +
		"Service/endpoints + connection string."
	e.mu.Lock()
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "signature-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries), "both owners in same signature group")
}

func TestSmartGroupingSignatureFallback(t *testing.T) {
	e := newSmartGroupingEngine()
	// No logs set → no signature match → owner-scoped fallback
	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep2",
		nil,
	)
	e.mu.Lock()
	_, has1 := e.groupBuffers["CrashLoopBackOff|ns|dep1"]
	_, has2 := e.groupBuffers["CrashLoopBackOff|ns|dep2"]
	_, hasSig := e.groupBuffers["CrashLoopBackOff|sig|"]
	e.mu.Unlock()
	assert.False(
		t,
		has1,
		"the first owner is announced immediately, not buffered",
	)
	assert.True(t, has2, "dep2 falls back to its owner-scoped key")
	assert.False(t, hasSig, "no signature-scoped group for empty logs")
}

func TestSmartGroupingImagePerImage(t *testing.T) {
	e := newSmartGroupingEngine()
	msg := "not found: nginx:latest"
	ev := event.Event{
		PodName: "p1", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.Process(ev, "dep1", nil)
	ev2 := event.Event{
		PodName: "p2", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.Process(ev2, "dep2", nil)

	e.mu.Lock()
	gk := "ImagePullBackOff|img|nginx:latest|ns|ns"
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "image-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries))
}

func TestSmartGroupingImageGlobal(t *testing.T) {
	e := newSmartGroupingEngine()
	msg := "toomanyrequests: rate limit"
	ev := event.Event{
		PodName: "p1", Namespace: "ns1", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.Process(ev, "dep1", nil)
	ev2 := event.Event{
		PodName: "p2", Namespace: "ns2", Reason: "ImagePullBackOff",
		Image: "alpine:latest", Message: msg,
	}
	e.Process(ev2, "dep2", nil)

	e.mu.Lock()
	gk := "ImagePullBackOff|global|rate_limit"
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "global rate_limit group must exist")
	// Both pods map to the same global key => single entry
	assert.Equal(t, 1, len(pg.entries))
}

func TestSmartGroupingImageAuth(t *testing.T) {
	e := newSmartGroupingEngine()
	msg := "unauthorized: authentication required"
	ev := event.Event{
		PodName: "p1", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.Process(ev, "dep1", nil)
	ev2 := event.Event{
		PodName: "p2", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "alpine:latest", Message: msg,
	}
	e.Process(ev2, "dep2", nil)

	e.mu.Lock()
	gk := "ImagePullBackOff|ns|ns"
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "auth ns-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries))
}

func TestSmartGroupingNamespaceScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CreateContainerConfigError",
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns2",
			Reason:    "CreateContainerConfigError",
		},
		"dep2",
		nil,
	)

	e.mu.Lock()
	_, has1 := e.groupBuffers["CreateContainerConfigError|ns|ns"]
	_, has2 := e.groupBuffers["CreateContainerConfigError|ns|ns2"]
	e.mu.Unlock()
	assert.True(t, has1, "ns group must exist")
	assert.True(t, has2, "ns2 group must exist")
}

func TestSmartGroupingCrossNamespace(t *testing.T) {
	e := newSmartGroupingEngine()
	// Each namespace has its own window, so each owner is the first in its
	// namespace and both alert immediately; nothing is buffered.
	_, a1 := e.Process(
		event.Event{PodName: "p1", Namespace: "ns1", Reason: "OOMKilled"},
		"dep1",
		nil,
	)
	_, a2 := e.Process(
		event.Event{PodName: "p2", Namespace: "ns2", Reason: "OOMKilled"},
		"dep1",
		nil,
	)
	assert.Equal(t, model.ActionCreate, a1)
	assert.Equal(t, model.ActionCreate, a2)
	e.mu.Lock()
	buffers := len(e.groupBuffers)
	e.mu.Unlock()
	assert.Equal(t, 0, buffers, "a lone owner per namespace is never buffered")
}

func TestSmartGroupingEntryLimit(t *testing.T) {
	e := newSmartGroupingEngine()
	sigLog := "connection refused:5432"
	gk := "CrashLoopBackOff|sig|Postgres unreachable — check the DB " +
		"Service/endpoints + connection string."

	for i := 0; i < 1002; i++ {
		ev := event.Event{
			PodName:   fmt.Sprintf("p%d", i),
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		}
		e.Process(ev, fmt.Sprintf("dep%d", i), nil)
	}

	e.mu.Lock()
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "pending group must exist")
	assert.Equal(t, maxGroupEntries, len(pg.entries), "entries must be capped")
	assert.Equal(
		t,
		2,
		pg.overflowCount,
		"1 entry from first overflow + 1 from second",
	)
}

func TestSmartGroupingSeverityInheritance(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(event.Event{
		PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff",
		Logs: sigLog, Severity: "normal",
	}, "dep1", nil)
	e.Process(event.Event{
		PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff",
		Logs: sigLog, Severity: "critical",
	}, "dep2", nil)

	var groupInc *model.Incident
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupInc = inc
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	require.NotNil(t, groupInc, "group summary must be emitted")
	assert.Equal(
		t,
		model.SeverityCritical,
		groupInc.Severity,
		"group must inherit highest severity",
	)
}

// --- mock service lister ---

type mockServiceLister struct {
	corev1lister.ServiceLister
	listFn func(ns string) ([]*corev1.Service, error)
}

func (m *mockServiceLister) Services(
	namespace string,
) corev1lister.ServiceNamespaceLister {
	return &mockSvcNsLister{listFn: func() ([]*corev1.Service, error) {
		return m.listFn(namespace)
	}}
}

type mockSvcNsLister struct {
	corev1lister.ServiceNamespaceLister
	listFn func() ([]*corev1.Service, error)
}

func (m *mockSvcNsLister) List(
	selector labels.Selector,
) ([]*corev1.Service, error) {
	return m.listFn()
}
func (m *mockSvcNsLister) Get(name string) (*corev1.Service, error) {
	return nil, nil
}

func TestFindDependentServicesNoLister(t *testing.T) {
	e := newTestEngine()
	got := e.findDependentServices("ns", map[string]string{"app": "myapp"})
	assert.Nil(t, got)
}

func TestFindDependentServicesNoLabels(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{})
	got := e.findDependentServices("ns", nil)
	assert.Nil(t, got)
}

func TestFindDependentServicesMatch(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
			}, nil
		},
	})
	got := e.findDependentServices("ns", map[string]string{"app": "api"})
	assert.Equal(t, []string{"svc-api"}, got)
}

func TestFindDependentServicesNoMatch(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
			}, nil
		},
	})
	got := e.findDependentServices("ns", map[string]string{"app": "web"})
	assert.Empty(t, got)
}

func TestFindDependentServicesMultiple(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-grpc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-other",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "other"},
					},
				},
			}, nil
		},
	})
	got := e.findDependentServices("ns", map[string]string{"app": "api"})
	assert.Len(t, got, 2)
	assert.Contains(t, got, "svc-api")
	assert.Contains(t, got, "svc-grpc")
}

func TestFindDependentServicesEmptySelector(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-headless",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{Selector: nil},
				},
			}, nil
		},
	})
	got := e.findDependentServices("ns", map[string]string{"app": "api"})
	assert.Empty(t, got)
}

// --- cascading suppression ---

func TestCascadingSuppressionSuppressesPodWhenDeploymentUnavailable(
	t *testing.T,
) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       2,
					UnavailableReplicas: 1,
				},
			}, nil
		},
	})

	// First, create a deployment-level incident
	depEv := event.Event{
		Resource:  "deployment",
		Namespace: "ns",
		PodName:   "myapp",
		Reason:    "DeploymentUnavailable",
	}
	depInc, depAction := e.Process(depEv, "myapp", nil)
	assert.Equal(t, model.ActionCreate, depAction)
	assert.NotNil(t, depInc)

	// Now process a pod incident for the same owner
	podEv := event.Event{
		Resource:      "pod",
		Namespace:     "ns",
		PodName:       "myapp-7d8f9-abc",
		ContainerName: "app",
		Reason:        "CrashLoopBackOff",
	}
	podInc, podAction := e.Process(
		podEv,
		"myapp",
		&model.ContainerState{RestartCount: 1},
	)
	assert.Equal(
		t,
		model.ActionSkip,
		podAction,
		"pod incident should be suppressed",
	)
	assert.Nil(t, podInc)

	// Verify the deployment incident was attributed
	assert.Equal(t, 2, e.state[depInc.Key].Count)
	assert.True(t, e.state[depInc.Key].Resources["myapp-7d8f9-abc"])
}

func TestCascadingSuppressionNoSuppressionWhenDeploymentHealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       3,
					UnavailableReplicas: 0,
				},
			}, nil
		},
	})

	// Create a pod incident (no parent incident exists)
	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-7d8f9-abc",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
}

func TestCascadingSuppressionNoSuppressionForDifferentOwner(t *testing.T) {
	e := newTestEngine()

	// Create deployment incident for owner "dep-a"
	depEv := event.Event{
		Resource:  "deployment",
		Namespace: "ns",
		PodName:   "dep-a",
		Reason:    "DeploymentUnavailable",
	}
	e.Process(depEv, "dep-a", nil)

	// Pod incident for different owner "dep-b"
	podEv := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "dep-b-xyz",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(podEv, "dep-b", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"different owner should not be suppressed",
	)
	assert.NotNil(t, inc)
}

func TestCascadingSuppressionNoSuppressionForResolvedParent(t *testing.T) {
	e := newTestEngine()

	// Create and resolve a deployment incident
	depEv := event.Event{
		Resource:  "deployment",
		Namespace: "ns",
		PodName:   "myapp",
		Reason:    "DeploymentUnavailable",
	}
	depInc, _ := e.Process(depEv, "myapp", nil)
	e.MarkResolved(depInc.Key)

	// Pod incident should not be suppressed (parent is resolved)
	podEv := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(podEv, "myapp", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"pod should alert when parent is resolved",
	)
	assert.NotNil(t, inc)
}

func TestNewIncidentAnnotatesDependentServices(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "myapp"},
					},
				},
			}, nil
		},
	})

	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
		Labels:    map[string]string{"app": "myapp"},
		OwnerKind: "Deployment",
	}
	inc, action := e.Process(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	// Topology is structured impact, not hint prose.
	assert.Equal(t, []string{"svc-api"}, inc.AffectedServices)
	assert.NotContains(
		t,
		inc.Hint,
		"affects service",
		"impact must not be folded into the hint",
	)
}

func TestNewIncidentAnnotatesParentUnhealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       2,
					UnavailableReplicas: 1,
				},
			}, nil
		},
	})

	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
		OwnerKind: "Deployment",
	}
	inc, action := e.Process(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.True(
		t,
		inc.OwnerUnhealthy,
		"the owner's health is structured, not hint prose",
	)
	assert.NotContains(t, inc.Hint, "also unhealthy")
}

func TestNewIncidentDoesNotAnnotateParentWhenHealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       3,
					UnavailableReplicas: 0,
				},
			}, nil
		},
	})

	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
		OwnerKind: "Deployment",
	}
	inc, action := e.Process(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.NotContains(t, inc.Hint, "owning")
}

func TestClassifyImagePullScope(t *testing.T) {
	tests := []struct {
		msg      string
		expected string
	}{
		{"toomanyrequests: pull limit", "rate_limit"},
		{"rate limit exceeded", "rate_limit"},
		{"pull qps exceeded", "pull_qps"},
		{"authentication required", "auth"},
		{"unauthorized: access denied", "auth"},
		{"denied: access forbidden", "auth"},
		{"no pull access", "auth"},
		{"not found: nginx:latest", "image_not_found"},
		{"manifest unknown", "image_not_found"},
		{"does not exist", "image_not_found"},
		{"context deadline exceeded", "timeout"},
		{"i/o timeout", "timeout"},
		{"connection refused", "conn_refused"},
		{"connection reset", "conn_refused"},
		{"no route to host", "net_unreachable"},
		{"network is unreachable", "net_unreachable"},
		{"no such host", "dns"},
		{"dial tcp: lookup registry.example.com", "dns"},
		{"tls handshake error", "tls"},
		{"certificate expired", "tls"},
		{"some random error", ""},
		{"", ""},
	}
	for _, tc := range tests {
		assert.Equal(
			t,
			tc.expected,
			classifyImagePullScope(tc.msg),
			"classifyImagePullScope(%q)",
			tc.msg,
		)
	}
}

func TestSeverityRank(t *testing.T) {
	assert.Equal(t, 3, model.SeverityCritical.Rank())
	assert.Equal(t, 2, model.SeverityHigh.Rank())
	assert.Equal(t, 1, model.SeverityMedium.Rank())
	assert.Equal(
		t,
		1,
		model.SeverityWarning.Rank(),
		"warning must rank above normal for sticky escalation",
	)
	assert.Equal(t, 0, model.SeverityNormal.Rank())
	assert.Equal(t, 0, model.Severity("").Rank())
	assert.Equal(t, 0, model.Severity("unknown").Rank())
}

func TestCountActiveNodeIncidents(t *testing.T) {
	e := newTestEngine()
	assert.Equal(t, 0, e.CountActiveNodeIncidents())

	e.SetActiveNodeIncidents([]string{"node-1", "node-2"})
	assert.Equal(t, 2, e.CountActiveNodeIncidents())

	e2 := newTestEngine()
	assert.Equal(t, 0, e2.CountActiveNodeIncidents())
}

func TestBuildNodeSummary(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{reason: "DiskPressure", nodeName: "node-1", podName: "p1"},
		{reason: "DiskPressure", nodeName: "node-1", podName: "p2"},
	}
	summary := e.buildNodeSummary(entries)
	assert.Equal(t, "2 pods on node node-1", summary)
}

func TestBuildImageSummaryPerImage(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{
			reason:    "ImagePullBackOff",
			image:     "nginx:latest",
			namespace: "ns",
			owner:     "dep1",
			key:       "ImagePullBackOff|img|nginx:latest|ns|ns",
		},
		{
			reason:    "ImagePullBackOff",
			image:     "nginx:latest",
			namespace: "ns",
			owner:     "dep2",
			key:       "ImagePullBackOff|img|nginx:latest|ns|ns",
		},
	}
	summary := e.buildImageSummary(entries)
	assert.Equal(t, "image nginx:latest — dep1, dep2", summary)
}

func TestBuildImageSummaryGlobal(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{
			reason:    "ImagePullBackOff",
			image:     "nginx:latest",
			namespace: "ns1",
			owner:     "dep1",
			key:       "ImagePullBackOff|global|rate_limit",
		},
		{
			reason:    "ImagePullBackOff",
			image:     "alpine:latest",
			namespace: "ns2",
			owner:     "dep2",
			key:       "ImagePullBackOff|global|rate_limit",
		},
	}
	summary := e.buildImageSummary(entries)
	assert.Equal(t, "2 workloads across 2 namespaces", summary)
	assert.NotContains(
		t,
		summary,
		"nginx",
		"a registry-wide failure is not about one image",
	)
}

func TestBuildImageSummaryEmptyImage(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{
			reason:    "ImagePullBackOff",
			image:     "",
			namespace: "ns",
			owner:     "dep1",
			key:       "img",
		},
	}
	summary := e.buildImageSummary(entries)
	assert.Contains(t, summary, "unknown image")
}

func TestSetSeverityMap(t *testing.T) {
	e := NewEngine(Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})
	sm := map[string]string{"CrashLoopBackOff": "critical"}
	e.SetSeverityMap(sm)

	// Engine with non-DefaultEnricher should not panic
	e2 := NewEngine(Config{Window: 10 * time.Minute})
	e2.SetSeverityMap(sm)
}
