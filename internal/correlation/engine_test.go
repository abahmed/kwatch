package correlation

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	e.config.StartupQuiet = 0

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

func TestClearSeenUnsuppresses(t *testing.T) {
	e := newTestEngine()
	e.config.StartupQuiet = 0

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
	e.config.StartupQuiet = 0
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
	e.config.StartupQuiet = 0
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
	e.config.StartupQuiet = 0

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

func TestStartupQuietSuppressesAllBeforeSeen(t *testing.T) {
	e := newTestEngine()
	e.config.StartupQuiet = 10 * time.Minute

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestStsOwnedPodsGroupByStsName(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
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
	assert.True(t, inc1.Resources["db-0"])
	assert.True(t, inc1.Resources["db-1"])
	assert.Equal(t, 2, inc1.Count)
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
		Window:                    10 * time.Minute,
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
	assert.Same(t, inc, inc2)
}

func TestEscalationSecondCrossingIsCritical(t *testing.T) {
	e := escTestEngine()
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 4}) // → high
	inc, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 11})
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
	_, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 50})
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
	}
}

// ── Storm tests ───────────────────────────────────────────────────

func stormEngine() *Engine {
	return NewEngine(Config{
		Window:               10 * time.Minute,
		StormEnabled:         true,
		StormThreshold:       3,
		StormWindow:          time.Minute,
		StormDigestInterval:  time.Nanosecond,
	})
}

func TestStormBuffersCreatesOverThreshold(t *testing.T) {
	e := stormEngine()
	var digests int
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if action == model.ActionDigestFlush {
			digests++
		}
	}
	for i := 0; i < 5; i++ {
		ev := event.Event{
			PodName:   fmt.Sprintf("p%d", i),
			Namespace: "ns",
			Reason:    fmt.Sprintf("R%d", i),
		}
		_, action := e.Process(ev, fmt.Sprintf("o%d", i), nil)
		if action == model.ActionDigest {
			digests++
		}
	}
	assert.GreaterOrEqual(t, digests, 1)
	assert.NotEmpty(t, e.digestBuf)
	assert.Equal(t, 5, len(e.state))
}

func TestStormFlushEmitsSummary(t *testing.T) {
	e := stormEngine()
	var flushActions int
	var lastDigest *model.Incident
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if action == model.ActionDigestFlush {
			flushActions++
			lastDigest = inc
		}
	}
	// fill storm buffer — use same reason so summary groups them
	for i := 0; i < 5; i++ {
		e.Process(event.Event{
			PodName: fmt.Sprintf("p%d", i), Namespace: "ns", Reason: "OOMKilled",
		}, fmt.Sprintf("o%d", i), nil)
	}
	// trigger lifecycle
	e.checkLifecycle()
	assert.GreaterOrEqual(t, flushActions, 1)
	if lastDigest != nil {
		// threshold=3, so last 3 creates are buffered
		assert.Equal(t, 3, lastDigest.Count)
		assert.Equal(t, "DigestSummary", lastDigest.Reason)
		assert.NotEmpty(t, lastDigest.Hint)
	}
}

func TestStormDisabledNeverDigests(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	var digests int
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if action == model.ActionDigestFlush || action == model.ActionDigest {
			digests++
		}
	}
	for i := 0; i < 20; i++ {
		e.Process(event.Event{
			PodName: fmt.Sprintf("p%d", i), Namespace: "ns", Reason: fmt.Sprintf("R%d", i),
		}, fmt.Sprintf("o%d", i), nil)
	}
	assert.Equal(t, 0, digests)
}

func TestStormResolvesStillNotify(t *testing.T) {
	e := stormEngine()
	var resolves int
	e.config.LifecycleHook = func(inc *model.Incident, action model.IncidentAction) {
		if action == model.ActionResolved {
			resolves++
		}
	}
	// create an incident then remove the pod
	ev := event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}
	e.Process(ev, "dep", nil)
	e.RemovePod("ns", "p1")
	assert.GreaterOrEqual(t, resolves, 1)
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
	e.config.StartupQuiet = 0

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

func TestStartupQuietPreseededSeenDoesNotSuppressNodeEvent(t *testing.T) {
	// Pre-seed seen with a pod key (simulates pod baseline from buildSeenSet).
	// During startup-quiet, a fresh node event must NOT be suppressed.
	e := NewEngine(Config{
		Window:       10 * time.Minute,
		StartupQuiet: 1 * time.Hour,
	})
	e.SetSeen(map[string]map[string]int64{"ns:dep:CrashLoopBackOff:": {"pod-1": time.Now().Unix()}})

	// The pod key makes len(seen) > 0, so the blanket quiet is disabled.
	// A node event should create, not skip.
	ev := event.Event{Resource: "node", PodName: "node-1", NodeName: "node-1", Reason: "NodeNotReady"}
	_, action := e.Process(ev, "node-1", nil)
	assert.Equal(t, model.ActionCreate, action,
		"node event should create during startup-quiet when pod baseline is present")
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
	e.config.StartupQuiet = 0

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// MarkResolved should NOT fire the hook immediately
	e.MarkResolved(inc.Key)
	assert.Equal(t, 0, resolves)
	assert.Equal(t, model.StatePendingResolve, inc.State)
	assert.Equal(t, fakeNow.Add(10*time.Minute), inc.ResolveAt)
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
	e.config.StartupQuiet = 0

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Pending resolve
	e.MarkResolved(inc.Key)
	assert.Equal(t, 0, resolves)
	assert.Equal(t, model.StatePendingResolve, inc.State)

	// Recurrence within cooldown — should revive (skip) and cancel the pending resolve
	inc2, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action, "revive within cooldown must skip, not update")
	assert.Equal(t, model.StateActive, inc2.State, "pending resolve must be revoked")
	assert.True(t, inc2.ResolveAt.IsZero(), "ResolveAt must be cleared")
	assert.Equal(t, 0, resolves, "hook must not fire")
}

func TestProcessResolvedIncidentCreatesFresh(t *testing.T) {
	e := newTestEngine()
	e.config.StartupQuiet = 0

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	key := inc.Key

	// Immediately resolve
	e.MarkResolved(key)
	assert.Equal(t, model.StateResolved, inc.State)

	// Process again — should create fresh (not update)
	inc2, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotEqual(t, inc, inc2, "must be a new incident object")
	assert.Equal(t, key, inc2.Key)
}

func TestIncidentKeyMatchesProcess(t *testing.T) {
	tests := []struct {
		name    string
		ev      event.Event
		owner   string
		cs      *model.ContainerState
	}{
		{
			name:    "CrashLoopBackOff with cs",
			ev:      event.Event{Namespace: "default", Reason: "CrashLoopBackOff"},
			owner:   "deploy-1",
			cs:      &model.ContainerState{RestartCount: 3},
		},
		{
			name:    "CrashLoopBackOff high frequency",
			ev:      event.Event{Namespace: "default", Reason: "CrashLoopBackOff"},
			owner:   "deploy-1",
			cs:      &model.ContainerState{RestartCount: 10},
		},
		{
			name:    "normalized reason",
			ev:      event.Event{Namespace: "default", Reason: "CrashLoopBackOff 42"},
			owner:   "deploy-1",
			cs:      &model.ContainerState{RestartCount: 1},
		},
		{
			name:    "empty container",
			ev:      event.Event{Namespace: "default", Reason: "OOMKilled"},
			owner:   "deploy-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := IncidentKey(tt.ev, tt.owner, tt.cs)

			e := newTestEngine()
			e.config.StartupQuiet = 0
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
	e.config.StartupQuiet = 0

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	e.MarkResolved(inc.Key)
	assert.Equal(t, model.StatePendingResolve, inc.State)

	time.Sleep(2 * time.Millisecond)
	e.checkLifecycle()

	assert.Equal(t, 1, resolved)
	assert.True(t, baselineChanged, "OnBaselineChange must fire when pending resolve finalizes")
	assert.Equal(t, model.StateResolved, inc.State)
}

func TestPerPodBaselineNewPodAlerts(t *testing.T) {
	e := newTestEngine()
	e.config.StartupQuiet = 0

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
	e.config.StartupQuiet = 0

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
	e.config.StartupQuiet = 0

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
	e.config.StartupQuiet = 0

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
	key := inc.Key

	// Resolve
	e.MarkResolved(key)
	assert.Equal(t, model.StateResolved, inc.State)

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
	e.config.StartupQuiet = 0

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Mark pending resolve
	e.MarkResolved(inc.Key)
	assert.Equal(t, model.StatePendingResolve, inc.State)
	assert.Equal(t, 0, resolved)

	// Revive → skip (edge-triggered, same sig), state back to active
	inc2, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
	assert.Equal(t, model.StateActive, inc2.State)
	assert.True(t, inc2.ResolveAt.IsZero())
	assert.Equal(t, 0, resolved, "no ActionResolved should be emitted")
}
