package correlation

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
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
	assert.Equal(t, "default:deploy-1:CrashLoopBackOff:", inc.Key)
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

	// Second event with identical sig → skip (edge-triggered), but Count still updates
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

	// Second event with same sig → skip (edge-triggered), Count and Resources still update
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
	assert.Equal(t, "default::OOMKilled:", inc.Key)
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

func TestRemovePodMultiIncidentResolve(t *testing.T) {
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

	var resolvedKeys []string
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if action == model.ActionResolved {
			resolvedKeys = append(resolvedKeys, inc.Key)
		}
	}

	e.RemovePod("default", "pod-1")

	assert.Equal(t, 2, len(resolvedKeys), "both incidents should resolve")
	assert.Equal(t, 0, len(e.state["default:deploy-1:CrashLoopBackOff:"].Resources))
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

	e.SetSeen(map[string]map[string]int64{incidentKey: {"pod-1": fakeNow.Unix()}})

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestSetSeenMergesNotReplaces(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newTestEngine()
	e.now = mockClock(fakeNow)

	key1 := BuildKey("default", "dep-1", "CrashLoopBackOff", "")
	key2 := BuildKey("default", "dep-1", "OOMKilled", "")
	key3 := BuildKey("default", "dep-2", "CrashLoopBackOff", "")

	// First call: seed key1 and key2
	e.SetSeen(map[string]map[string]int64{
		key1: {"pod-a": fakeNow.Unix()},
		key2: {"pod-b": fakeNow.Unix()},
	})

	// Second call: same key1 with fresher timestamp, plus new key3
	later := fakeNow.Add(1 * time.Hour)
	e.SetSeen(map[string]map[string]int64{
		key1: {"pod-a": later.Unix()},
		key3: {"pod-c": later.Unix()},
	})

	// All keys should be present (key1 and key2 preserved from first call,
	// key3 from second call, key1 timestamp updated)
	e.mu.Lock()
	defer e.mu.Unlock()

	_, ok1 := e.seen[key1]["pod-a"]
	assert.True(t, ok1, "key1 from first SetSeen must survive after second SetSeen")

	_, ok2 := e.seen[key2]["pod-b"]
	assert.True(t, ok2, "key2 from first SetSeen must survive after second SetSeen (merge)")

	_, ok3 := e.seen[key3]["pod-c"]
	assert.True(t, ok3, "key3 from second SetSeen must be present")

	// Timestamp for key1/pod-a must reflect the later value (was updated, not stuck)
	assert.Equal(t, later.Unix(), e.seen[key1]["pod-a"],
		"SetSeen must update timestamp for existing entry")
}

func TestClearSeenUnsuppresses(t *testing.T) {
	e := newTestEngine()

	incidentKey := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")

	e.SetSeen(map[string]map[string]int64{incidentKey: {"pod-1": time.Now().Unix()}})
	e.ClearSeenForPod("default", "pod-1")

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
	e.SetSeen(map[string]map[string]int64{incidentKey: {"pod-1": fakeNow.Add(-1 * time.Hour).Unix()}})

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
	e.SetSeen(map[string]map[string]int64{incidentKey: {"pod-1": fakeNow.Add(-25 * time.Hour).Unix()}})

	ev := event.Event{
		PodName: "pod-1", Namespace: "default", Reason: "CrashLoopBackOff",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// entry should be pruned from seen
	e.mu.Lock()
	_, stillSeen := e.seen[incidentKey]
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
	e.SetSeen(map[string]map[string]int64{incidentKey: {"pod-1": time.Now().Unix()}})

	// RemovePod should clear the baseline when the incident empties
	e.RemovePod("default", "pod-1")

	// A new event for the same key should now fire (update, since the resolved
	// incident is still in state and gets reactivated)
	ev2 := event.Event{
		PodName:   "pod-2",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestStsOwnedPodsGroupByStsName(t *testing.T) {
	e := NewEngine(Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{SeverityByOwnerKind: map[string]string{"StatefulSet": "high"}},
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
	assert.Equal(t, "high", inc1.Severity)
	// After the second call, the live incident has both pods. Use inc2 (clone of the second call).
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
		Window:                     10 * time.Minute,
		RenotifyIntervalBySeverity: map[string]time.Duration{"default": 1 * time.Minute},
		RenotifyMaxPerIncident:     3,
	})
	if v := e.config.RenotifyIntervalBySeverity["default"]; v != 1*time.Minute {
		t.Errorf("unexpected renotify interval: %v", v)
	}
	if e.config.RenotifyMaxPerIncident != 3 {
		t.Errorf("unexpected renotify max: %d", e.config.RenotifyMaxPerIncident)
	}
}

// ── BUG-1: escalation ──────────────────────────────────────────────

func escTestEngine() *Engine {
	return NewEngine(Config{
		Window:            10 * time.Minute,
		EscalationEnabled: true,
		EscalationTiers:   []int{3, 10, 50},
	})
}

func TestEscalationFirstCrossingIsHigh(t *testing.T) {
	e := escTestEngine()
	// Use OOMKilled to avoid CrashLoopHighFrequency rename when RestartCount > 5
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	// within cooldown, cross tier 3:
	inc2, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 4})
	assert.Equal(t, model.ActionUpdate, action)
	assert.Equal(t, "high", inc2.Severity)
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
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})                // crosses tier 1 → high
	inc, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 4}) // crosses tier 3 → critical
	assert.Equal(t, model.ActionUpdate, action)
	assert.Equal(t, "critical", inc.Severity)
}

func TestEscalationSameTierSkips(t *testing.T) {
	e := escTestEngine()
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 4})
	_, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 5}) // 4→5: no tier, same sig
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
	nodeEv := event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}
	e.Process(nodeEv, "node-1", nil)
	podEv := event.Event{PodName: "p", Namespace: "ns", NodeName: "node-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(podEv, "dep", nil)
	assert.Nil(t, inc)
	assert.Equal(t, model.ActionSkip, action)
}

func TestInhibitionFlagOffDoesNotSuppress(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: false,
	})
	e.Process(event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}, "node-1", nil)
	_, action := e.Process(event.Event{PodName: "p", Namespace: "ns", NodeName: "node-1", Reason: "CrashLoopBackOff"}, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionOtherNodeUnaffected(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}, "node-1", nil)
	podEv := event.Event{PodName: "p", Namespace: "ns", NodeName: "node-2", Reason: "CrashLoopBackOff"}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionLiftsOnNodeResolve(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}, "node-1", nil)
	e.ResolveByResource("node", "node-1")
	podEv := event.Event{PodName: "p", Namespace: "ns", NodeName: "node-1", Reason: "CrashLoopBackOff"}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionSuppressedCounter(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}, "node-1", nil)
	e.Process(event.Event{PodName: "p1", Namespace: "ns", NodeName: "node-1", Reason: "CrashLoopBackOff"}, "dep", nil)
	nodeInc := e.findNodeIncident("node-1")
	if nodeInc != nil {
		assert.Equal(t, 1, nodeInc.SuppressedPods)
		if assert.NotNil(t, nodeInc.SuppressedOwners) {
			assert.Equal(t, 1, nodeInc.SuppressedOwners["dep"])
		}
	}
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

	ev := event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)

	// First MarkResolved should fire the hook
	e.MarkResolved(inc.Key)
	assert.Equal(t, 1, resolves)

	// Second MarkResolved (same key) must NOT fire again
	e.MarkResolved(inc.Key)
	assert.Equal(t, 1, resolves, "MarkResolved must be idempotent — hook fired twice")
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

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
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

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Pending resolve
	e.MarkResolved(inc.Key)
	assert.Equal(t, 0, resolves)
	live := e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StatePendingResolve, live.State)
	}

	// Recurrence within cooldown — should revive (skip) and cancel the pending resolve
	_, action = e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action, "revive within cooldown must skip, not update")
	live2 := e.state[inc.Key]
	if live2 != nil {
		assert.Equal(t, model.StateActive, live2.State, "pending resolve must be revoked")
		assert.True(t, live2.ResolveAt.IsZero(), "ResolveAt must be cleared")
	}
	assert.Equal(t, 0, resolves, "hook must not fire")
}

func TestProcessResolvedIncidentCreatesFresh(t *testing.T) {
	e := newTestEngine()

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	key := inc.Key

	// Immediately resolve
	e.MarkResolved(key)
	live := e.state[key]
	if live != nil {
		assert.Equal(t, model.StateResolved, live.State)
	}

	// Process again — should create fresh (not update)
	inc2, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, key, inc2.Key)
}

func TestIncidentKeyMatchesProcess(t *testing.T) {
	tests := []struct {
		name  string
		ev    event.Event
		owner string
		cs    *model.ContainerState
	}{
		{
			name:  "CrashLoopBackOff with cs",
			ev:    event.Event{Namespace: "default", Reason: "CrashLoopBackOff"},
			owner: "deploy-1",
			cs:    &model.ContainerState{RestartCount: 3},
		},
		{
			name:  "CrashLoopBackOff high frequency",
			ev:    event.Event{Namespace: "default", Reason: "CrashLoopBackOff"},
			owner: "deploy-1",
			cs:    &model.ContainerState{RestartCount: 10},
		},
		{
			name:  "normalized reason",
			ev:    event.Event{Namespace: "default", Reason: "CrashLoopBackOff 42"},
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

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
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
	assert.True(t, baselineChanged, "OnBaselineChange must fire when pending resolve finalizes")
	live = e.state[inc.Key]
	if live != nil {
		assert.Equal(t, model.StateResolved, live.State)
	}
}

func TestPerPodBaselineNewPodAlerts(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetSeen(map[string]map[string]int64{key: {"pod-1": time.Now().Unix()}})

	// pod-1 is baselined — should skip
	ev1 := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	_, action := e.Process(ev1, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)

	// pod-2 is new — should alert
	ev2 := event.Event{Namespace: "default", PodName: "pod-2", Reason: "CrashLoopBackOff"}
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestClearSeenForPodIsPerPod(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetSeen(map[string]map[string]int64{key: {"pod-1": time.Now().Unix(), "pod-2": time.Now().Unix()}})

	e.ClearSeenForPod("default", "pod-1")

	// pod-1 un-baselined → create
	ev1 := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	_, action := e.Process(ev1, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// pod-2 still baselined → skip
	ev2 := event.Event{Namespace: "default", PodName: "pod-2", Reason: "CrashLoopBackOff"}
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestRemovePodReleasesBaseline(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetSeen(map[string]map[string]int64{key: {"pod-1": time.Now().Unix()}})

	// RemovePod should release the baseline slot for pod-1
	e.RemovePod("default", "pod-1")

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestResolvedIncidentRecreatesOnce(t *testing.T) {
	e := newTestEngine()

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	key := inc.Key

	// Resolve
	e.MarkResolved(key)
	live := e.state[key]
	if live != nil {
		assert.Equal(t, model.StateResolved, live.State)
	}

	// First recurrence → ActionCreate and stored
	ev2 := event.Event{Namespace: "default", PodName: "pod-2", Reason: "CrashLoopBackOff"}
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Second recurrence within cooldown → ActionSkip (cooldown on the new incident)
	_, action = e.Process(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action, "must respect cooldown on the re-created incident, NOT re-create again")
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

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
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

	cs := &model.ContainerState{RestartCount: 3, Reason: "CrashLoopBackOff", Status: "waiting"}
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	e.Process(ev, "deploy-1", cs)

	key := "default/pod-1"
	assert.Contains(t, e.lastContainerIndex, key)
	assert.NotNil(t, e.lastContainerIndex[key])
	assert.Equal(t, int32(3), e.lastContainerIndex[key].RestartCount)

	before := len(e.lastContainerIndex)
	e.RemovePod("default", "pod-1")

	assert.NotContains(t, e.lastContainerIndex, key)
	assert.Equal(t, before-1, len(e.lastContainerIndex))
	assert.Nil(t, e.GetLastContainerState("default", "pod-1", "."))
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
	e.SetSeen(map[string]map[string]int64{incidentKey: {"node-1": fakeNow.Unix()}})

	// Node event with same key — should NOT be baselined
	ev := event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}
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
	podEv := event.Event{PodName: "p1", Namespace: "ns", NodeName: "node-1", Reason: "CrashLoopBackOff"}
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
	ev := event.Event{PodName: "pod-1", Namespace: "ns", Reason: "CrashLoopBackOff"}
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

func TestCleanupCooldownExpires(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)

	// Create incident
	ev := event.Event{PodName: "pod-1", Namespace: "ns", Reason: "CrashLoopBackOff"}
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
	e.Process(event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}, "node-1", nil)

	// Suppress pods from different owners on the same node
	e.Process(event.Event{PodName: "p1", Namespace: "ns", NodeName: "node-1", Reason: "CrashLoopBackOff"}, "deploy-1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns", NodeName: "node-1", Reason: "OOMKilled"}, "deploy-1", nil)
	e.Process(event.Event{PodName: "p3", Namespace: "ns", NodeName: "node-1", Reason: "CrashLoopBackOff"}, "statefulset-1", nil)

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
	e.Process(event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}, "node-1", nil)

	// Unschedulable pod (empty NodeName) — should be suppressed
	ev := event.Event{PodName: "p1", Namespace: "ns", NodeName: "", Reason: "Unschedulable"}
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

func (m *mockDeployLister) Deployments(namespace string) appsv1lister.DeploymentNamespaceLister {
	return &mockDeployNsLister{getFn: func(name string) (*appsv1.Deployment, error) {
		return m.getFn(namespace, name)
	}}
}

type mockDeployNsLister struct {
	appsv1lister.DeploymentNamespaceLister
	getFn func(name string) (*appsv1.Deployment, error)
}

func (m *mockDeployNsLister) Get(name string) (*appsv1.Deployment, error) {
	return m.getFn(name)
}
func (m *mockDeployNsLister) List(selector labels.Selector) ([]*appsv1.Deployment, error) {
	return nil, nil
}

type mockSSLister struct {
	appsv1lister.StatefulSetLister
	getFn func(ns, name string) (*appsv1.StatefulSet, error)
}

func (m *mockSSLister) StatefulSets(namespace string) appsv1lister.StatefulSetNamespaceLister {
	return &mockSSNsLister{getFn: func(name string) (*appsv1.StatefulSet, error) {
		return m.getFn(namespace, name)
	}}
}

type mockSSNsLister struct {
	appsv1lister.StatefulSetNamespaceLister
	getFn func(name string) (*appsv1.StatefulSet, error)
}

func (m *mockSSNsLister) Get(name string) (*appsv1.StatefulSet, error) {
	return m.getFn(name)
}
func (m *mockSSNsLister) List(selector labels.Selector) ([]*appsv1.StatefulSet, error) {
	return nil, nil
}

type mockDSLister struct {
	appsv1lister.DaemonSetLister
	getFn func(ns, name string) (*appsv1.DaemonSet, error)
}

func (m *mockDSLister) DaemonSets(namespace string) appsv1lister.DaemonSetNamespaceLister {
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
func (m *mockDSNsLister) List(selector labels.Selector) ([]*appsv1.DaemonSet, error) {
	return nil, nil
}

func TestSnapshotAllRestoreIncidentsRoundTrip(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{PodName: "pod-1", Namespace: "ns", Reason: "CrashLoopBackOff"}
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
		Window:   10 * time.Minute,
		Baseline: map[string]map[string]int64{inc.Key: {"pod-1": time.Now().Unix()}},
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

func TestSnapshotAllEmpty(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	snap := e.SnapshotAll()
	assert.NotNil(t, snap)
	assert.Equal(t, 0, len(snap))
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
		Window:   10 * time.Minute,
		Baseline: map[string]map[string]int64{inc.Key: {"pod-1": time.Now().Unix()}},
	})
	e2.RestoreIncidents(snap)

	e2.mu.Lock()
	restored, exists := e2.state[inc.Key]
	e2.mu.Unlock()
	assert.True(t, exists, "restored incident must exist in state")
	assert.True(t, restored.LastSeen.After(originalLastSeen),
		"expected restored LastSeen (%v) to be after original (%v)", restored.LastSeen, originalLastSeen)
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
		Namespace: "ns",
		Name:      "my-deploy",
		OwnerKind: "Deployment",
		Resource:  "pod",
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

	inc := &model.Incident{Namespace: "ns", Name: "my-deploy", OwnerKind: "Deployment", Resource: "pod"}
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

	inc := &model.Incident{Namespace: "ns", Name: "my-deploy", OwnerKind: "Deployment", Resource: "pod"}
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
	inc := &model.Incident{Namespace: "ns", Name: "my-deploy", OwnerKind: "Deployment", Resource: "pod", Resources: map[string]bool{"p": true}}
	assert.False(t, e.isOwnerHealthy(inc))

	// Without resources → healthy (safe to resolve)
	inc2 := &model.Incident{Namespace: "ns", Name: "my-deploy", OwnerKind: "Deployment", Resource: "pod"}
	assert.True(t, e.isOwnerHealthy(inc2))
}

func TestIsOwnerHealthyNonPodResource(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})

	inc := &model.Incident{Namespace: "ns", Name: "my-node", OwnerKind: "", Resource: "node"}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyNilListers(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})

	inc := &model.Incident{Namespace: "ns", Name: "my-deploy", OwnerKind: "Deployment", Resource: "pod"}
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

	inc := &model.Incident{Namespace: "ns", Name: "my-ss", OwnerKind: "StatefulSet", Resource: "pod"}
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

	inc := &model.Incident{Namespace: "ns", Name: "my-ds", OwnerKind: "DaemonSet", Resource: "pod"}
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

	inc := &model.Incident{Namespace: "ns", Name: "my-ds", OwnerKind: "DaemonSet", Resource: "pod"}
	assert.False(t, e.isOwnerHealthy(inc))
}

func TestClearSeenForPodClearsCooldown(t *testing.T) {
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

	// ClearSeenForPod for the pod's namespace
	e.ClearSeenForPod("ns", "pod-1")

	// Cooldown should be cleared
	e.mu.Lock()
	_, exists := e.cleanupCooldown[key]
	e.mu.Unlock()
	assert.False(t, exists, "cooldown should be cleared by ClearSeenForPod")
}

// ── Smart grouping (reason-adaptive) tests ─────────────────────────

func newSmartGroupingEngine() *Engine {
	return NewEngine(Config{
		Window:              10 * time.Minute,
		SmartGroupingWindow: 60 * time.Second,
	})
}

func TestSmartGroupingBuffersSameReason(t *testing.T) {
	e := newSmartGroupingEngine()

	_, action := e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)
	assert.Equal(t, model.ActionSkip, action)
	assert.Equal(t, 1, len(e.state), "incident must still be added to state")

	_, action = e.Process(event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep2", nil)
	assert.Equal(t, model.ActionSkip, action)
	assert.Equal(t, 2, len(e.state))

	var hooks int
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) { hooks++ }
	e.checkLifecycle()
	assert.Equal(t, 0, hooks, "no hooks before window expiry")
}

func TestSmartGroupingFlushAfterWindow(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff", Logs: sigLog}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff", Logs: sigLog}, "dep2", nil)

	var groupInc *model.Incident
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if strings.HasPrefix(inc.Key, "__group__") {
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
	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns", Reason: "OOMKilled"}, "dep1", nil)

	var groups int
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if strings.HasPrefix(inc.Key, "__group__") {
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

	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep2", nil)

	e.MarkResolved("ns:dep1:CrashLoopBackOff:")

	var groupCount int
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if strings.HasPrefix(inc.Key, "__group__") {
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
		{namespace: "ns", owner: "dep1", reason: "CrashLoopBackOff", podName: "p1"},
		{namespace: "ns", owner: "dep2", reason: "CrashLoopBackOff", podName: "p2"},
		{namespace: "ns", owner: "dep1", reason: "CrashLoopBackOff", podName: "p3"},
	}
	summary := e.buildGroupSummary(entries, now)
	assert.Contains(t, summary, "CrashLoopBackOff")
	assert.Contains(t, summary, "total 3")
	assert.Contains(t, summary, "dep1")
	assert.Contains(t, summary, "dep2")
}

func TestBuildGroupSummaryEmpty(t *testing.T) {
	e := newSmartGroupingEngine()
	assert.Equal(t, "", e.buildGroupSummary(nil, time.Time{}))
	assert.Equal(t, "", e.buildGroupSummary([]groupEntry{}, time.Time{}))
}

func TestSmartGroupingWindowConfigZeroDisabled(t *testing.T) {
	e := NewEngine(Config{
		Window:              10 * time.Minute,
		SmartGroupingWindow: 0,
	})
	_, action := e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)
	assert.Equal(t, model.ActionCreate, action, "window=0 should disable buffering")
}

func TestSmartGroupingPendingGroupCleanedAfterFlush(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	e.mu.Lock()
	pg, exists := e.pendingGroups["CrashLoopBackOff|ns|dep1"]
	e.mu.Unlock()
	assert.False(t, exists, "pending group must be deleted after flush")
	require.Nil(t, pg)
}

func TestSmartGroupingIncidentHasNotifiedSig(t *testing.T) {
	e := newSmartGroupingEngine()
	inc, action := e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)
	assert.Equal(t, model.ActionSkip, action)
	require.NotNil(t, inc)
	assert.NotZero(t, inc.NotifiedSig, "NotifiedSig must be set")
	assert.NotZero(t, inc.LastNotifiedAt, "LastNotifiedAt must be set")
}

// ── Reason-adaptive scope tests ────────────────────────────────────

func TestSmartGroupingOwnerScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "OOMKilled"}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns", Reason: "OOMKilled"}, "dep2", nil)

	e.mu.Lock()
	_, has1 := e.pendingGroups["OOMKilled|ns|dep1"]
	_, has2 := e.pendingGroups["OOMKilled|ns|dep2"]
	e.mu.Unlock()
	assert.True(t, has1, "dep1 owner-scoped group must exist")
	assert.True(t, has2, "dep2 owner-scoped group must exist")
}

func TestSmartGroupingNodeScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(event.Event{PodName: "node-1", Resource: "node", NodeName: "node-1", Reason: "DiskPressure"}, "node-1", nil)
	e.Process(event.Event{PodName: "node-2", Resource: "node", NodeName: "node-2", Reason: "DiskPressure"}, "node-2", nil)

	e.mu.Lock()
	_, has1 := e.pendingGroups["DiskPressure|node|node-1"]
	_, has2 := e.pendingGroups["DiskPressure|node|node-2"]
	e.mu.Unlock()
	assert.True(t, has1, "node-1 group must exist")
	assert.True(t, has2, "node-2 group must exist")
}

func TestSmartGroupingSignatureScope(t *testing.T) {
	e := newSmartGroupingEngine()
	sigLog := "connection refused:5432"
	e.Process(event.Event{PodName: "p1", Namespace: "ns1", Reason: "CrashLoopBackOff", Logs: sigLog}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns2", Reason: "CrashLoopBackOff", Logs: sigLog}, "dep2", nil)

	gk := "CrashLoopBackOff|sig|Postgres unreachable — check the DB Service/endpoints + connection string."
	e.mu.Lock()
	pg, ok := e.pendingGroups[gk]
	e.mu.Unlock()
	require.True(t, ok, "signature-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries), "both owners in same signature group")
}

func TestSmartGroupingSignatureFallback(t *testing.T) {
	e := newSmartGroupingEngine()
	// No logs set → no signature match → owner-scoped fallback
	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep2", nil)

	e.mu.Lock()
	_, has1 := e.pendingGroups["CrashLoopBackOff|ns|dep1"]
	_, has2 := e.pendingGroups["CrashLoopBackOff|ns|dep2"]
	_, hasSig := e.pendingGroups["CrashLoopBackOff|sig|"]
	e.mu.Unlock()
	assert.True(t, has1, "dep1 owner-scoped fallback must exist")
	assert.True(t, has2, "dep2 owner-scoped fallback must exist")
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
	pg, ok := e.pendingGroups[gk]
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
	pg, ok := e.pendingGroups[gk]
	e.mu.Unlock()
	require.True(t, ok, "global rate_limit group must exist")
	assert.Equal(t, 2, len(pg.entries))
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
	pg, ok := e.pendingGroups[gk]
	e.mu.Unlock()
	require.True(t, ok, "auth ns-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries))
}

func TestSmartGroupingNamespaceScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CreateContainerConfigError"}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns2", Reason: "CreateContainerConfigError"}, "dep2", nil)

	e.mu.Lock()
	_, has1 := e.pendingGroups["CreateContainerConfigError|ns|ns"]
	_, has2 := e.pendingGroups["CreateContainerConfigError|ns|ns2"]
	e.mu.Unlock()
	assert.True(t, has1, "ns group must exist")
	assert.True(t, has2, "ns2 group must exist")
}

func TestSmartGroupingCrossNamespace(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(event.Event{PodName: "p1", Namespace: "ns1", Reason: "OOMKilled"}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns2", Reason: "OOMKilled"}, "dep1", nil)

	e.mu.Lock()
	_, has1 := e.pendingGroups["OOMKilled|ns1|dep1"]
	_, has2 := e.pendingGroups["OOMKilled|ns2|dep1"]
	e.mu.Unlock()
	assert.True(t, has1, "ns1 group must exist")
	assert.True(t, has2, "ns2 group must exist")
}

func TestSmartGroupingEntryLimit(t *testing.T) {
	e := newSmartGroupingEngine()
	sigLog := "connection refused:5432"
	gk := "CrashLoopBackOff|sig|Postgres unreachable — check the DB Service/endpoints + connection string."

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
	pg, ok := e.pendingGroups[gk]
	e.mu.Unlock()
	require.True(t, ok, "pending group must exist")
	assert.Equal(t, maxGroupEntries, len(pg.entries), "entries must be capped")
	assert.Equal(t, 2, pg.overflowCount, "1 entry from first overflow + 1 from second")
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
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if strings.HasPrefix(inc.Key, "__group__") {
			groupInc = inc
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	require.NotNil(t, groupInc, "group summary must be emitted")
	assert.Equal(t, "critical", groupInc.Severity, "group must inherit highest severity")
}

// --- mock service lister ---

type mockServiceLister struct {
	corev1lister.ServiceLister
	listFn func(ns string) ([]*corev1.Service, error)
}

func (m *mockServiceLister) Services(namespace string) corev1lister.ServiceNamespaceLister {
	return &mockSvcNsLister{listFn: func() ([]*corev1.Service, error) {
		return m.listFn(namespace)
	}}
}

type mockSvcNsLister struct {
	corev1lister.ServiceNamespaceLister
	listFn func() ([]*corev1.Service, error)
}

func (m *mockSvcNsLister) List(selector labels.Selector) ([]*corev1.Service, error) {
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
					ObjectMeta: metav1.ObjectMeta{Name: "svc-api", Namespace: "ns"},
					Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
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
					ObjectMeta: metav1.ObjectMeta{Name: "svc-api", Namespace: "ns"},
					Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
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
				{ObjectMeta: metav1.ObjectMeta{Name: "svc-api", Namespace: "ns"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "svc-grpc", Namespace: "ns"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "svc-other", Namespace: "ns"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "other"}}},
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
				{ObjectMeta: metav1.ObjectMeta{Name: "svc-headless", Namespace: "ns"}, Spec: corev1.ServiceSpec{Selector: nil}},
			}, nil
		},
	})
	got := e.findDependentServices("ns", map[string]string{"app": "api"})
	assert.Empty(t, got)
}

// --- cascading suppression ---

func TestCascadingSuppressionSuppressesPodWhenDeploymentUnavailable(t *testing.T) {
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
	podInc, podAction := e.Process(podEv, "myapp", &model.ContainerState{RestartCount: 1})
	assert.Equal(t, model.ActionSkip, podAction, "pod incident should be suppressed")
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
	assert.Equal(t, model.ActionCreate, action, "different owner should not be suppressed")
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
	assert.Equal(t, model.ActionCreate, action, "pod should alert when parent is resolved")
	assert.NotNil(t, inc)
}

func TestNewIncidentAnnotatesDependentServices(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{ObjectMeta: metav1.ObjectMeta{Name: "svc-api", Namespace: "ns"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "myapp"}}},
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
	assert.Contains(t, inc.Hint, "affects service(s): svc-api")
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
	assert.Contains(t, inc.Hint, "owning Deployment myapp is also unhealthy")
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
		assert.Equal(t, tc.expected, classifyImagePullScope(tc.msg), "classifyImagePullScope(%q)", tc.msg)
	}
}

func TestSeverityRank(t *testing.T) {
	assert.Equal(t, 3, severityRank("critical"))
	assert.Equal(t, 2, severityRank("high"))
	assert.Equal(t, 1, severityRank("medium"))
	assert.Equal(t, 0, severityRank("normal"))
	assert.Equal(t, 0, severityRank(""))
	assert.Equal(t, 0, severityRank("unknown"))
}

func TestBaselineSnapshot(t *testing.T) {
	e := NewEngine(Config{
		Window:      10 * time.Minute,
		BaselineTTL: 10 * time.Minute,
	})
	snap := e.BaselineSnapshot()
	assert.NotNil(t, snap)

	ts := time.Now().Unix()
	e.SetSeen(map[string]map[string]int64{
		"ns:dep:Err:": {"sig-1": ts},
	})
	snap = e.BaselineSnapshot()
	assert.Equal(t, ts, snap["ns:dep:Err:"]["sig-1"])

	// Verify isolation: mutate returned map's inner entry
	snap["ns:dep:Err:"]["sig-1"] = 999
	snap2 := e.BaselineSnapshot()
	assert.Equal(t, ts, snap2["ns:dep:Err:"]["sig-1"])
}

func TestCountActiveNodeIncidents(t *testing.T) {
	e := newTestEngine()
	assert.Equal(t, 0, e.CountActiveNodeIncidents())

	e.SetActiveNodeIncidents([]string{"node-1", "node-2"})
	assert.Equal(t, 2, e.CountActiveNodeIncidents())

	e2 := newTestEngine()
	assert.Equal(t, 0, e2.CountActiveNodeIncidents())
}

func TestSetAnalysis(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}
	inc, _ := e.Process(ev, "dep1", nil)

	e.SetAnalysis(inc.Key, "root cause found")
	e.mu.Lock()
	assert.Equal(t, "root cause found", e.state[inc.Key].Analysis)
	e.mu.Unlock()

	// No-op for non-existent key
	e.SetAnalysis("nonexistent", "should not panic")
}

func TestGetIncidentsByNamespace(t *testing.T) {
	e := newTestEngine()
	e.Process(event.Event{PodName: "p1", Namespace: "ns-a", Reason: "Err1"}, "dep1", nil)
	e.Process(event.Event{PodName: "p2", Namespace: "ns-b", Reason: "Err2"}, "dep2", nil)

	nsA := e.GetIncidentsByNamespace("ns-a")
	assert.Len(t, nsA, 1)
	assert.Equal(t, "Err1", nsA[0].Reason)

	nsB := e.GetIncidentsByNamespace("ns-b")
	assert.Len(t, nsB, 1)
	assert.Equal(t, "Err2", nsB[0].Reason)

	nsC := e.GetIncidentsByNamespace("ns-c")
	assert.Len(t, nsC, 0)
}

func TestBuildNodeSummary(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{reason: "DiskPressure", nodeName: "node-1", podName: "p1"},
		{reason: "DiskPressure", nodeName: "node-1", podName: "p2"},
	}
	summary := e.buildNodeSummary("DiskPressure", entries, " 5m ago")
	assert.Contains(t, summary, "DiskPressure")
	assert.Contains(t, summary, "node-1")
	assert.Contains(t, summary, "2 pods")
	assert.Contains(t, summary, "(total 2)")
	assert.Contains(t, summary, "5m ago")
}

func TestBuildImageSummaryPerImage(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{reason: "ImagePullBackOff", image: "nginx:latest", namespace: "ns", owner: "dep1", key: "ImagePullBackOff|img|nginx:latest|ns|ns"},
		{reason: "ImagePullBackOff", image: "nginx:latest", namespace: "ns", owner: "dep2", key: "ImagePullBackOff|img|nginx:latest|ns|ns"},
	}
	summary := e.buildImageSummary("ImagePullBackOff", entries, " 2m ago")
	assert.Contains(t, summary, "nginx")
	assert.Contains(t, summary, "dep1")
	assert.Contains(t, summary, "dep2")
	assert.Contains(t, summary, "(total 2)")
}

func TestBuildImageSummaryGlobal(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{reason: "ImagePullBackOff", image: "nginx:latest", namespace: "ns1", owner: "dep1", key: "ImagePullBackOff|global|rate_limit"},
		{reason: "ImagePullBackOff", image: "alpine:latest", namespace: "ns2", owner: "dep2", key: "ImagePullBackOff|global|rate_limit"},
	}
	summary := e.buildImageSummary("ImagePullBackOff", entries, " 2m ago")
	assert.Contains(t, summary, "2 deployments")
	assert.Contains(t, summary, "(total 2)")
	assert.NotContains(t, summary, "nginx")
}

func TestBuildImageSummaryEmptyImage(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{reason: "ImagePullBackOff", image: "", namespace: "ns", owner: "dep1", key: "img"},
	}
	summary := e.buildImageSummary("ImagePullBackOff", entries, "")
	assert.Contains(t, summary, "unknown")
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
