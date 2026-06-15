package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// defaultStormConfig returns a Config suitable for load tests that exercise
// storm collapse.
func defaultStormConfig(rec *recordingAlertManager) correlation.Config {
	return correlation.Config{
		Window:              10 * time.Minute,
		LifecycleInterval:   1 * time.Minute,
		StartupQuiet:        0,
		ResolveHoldDown:     0,
		Enricher:            &enricher.DefaultEnricher{},
		StormEnabled:        true,
		StormThreshold:      10,
		StormWindow:         5 * time.Minute,
		StormDigestInterval: 5 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			rec.NotifyIncident(inc, action)
		},
	}
}

// alertEntry holds a single (incident, action) notification captured by the
// recording alert manager.
type alertEntry struct {
	inc    *model.Incident
	action model.IncidentAction
}

// recordingAlertManager captures (incident, action) pairs for assertion in
// integration tests. It stands in for the real alert.AlertManager when
// wired through the correlation engine's LifecycleHook.
type recordingAlertManager struct {
	mu       sync.Mutex
	notified []alertEntry
}

func (r *recordingAlertManager) NotifyIncident(inc *model.Incident, action model.IncidentAction) {
	r.mu.Lock()
	r.notified = append(r.notified, alertEntry{inc: inc, action: action})
	r.mu.Unlock()
}

func (r *recordingAlertManager) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.notified)
}

func (r *recordingAlertManager) Get(i int) (*model.Incident, model.IncidentAction) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < 0 || i >= len(r.notified) {
		return nil, model.ActionSkip
	}
	return r.notified[i].inc, r.notified[i].action
}

// newTestEngine returns a correlation.Engine configured for deterministic
// integration testing: no startup quiet period, no resolve hold-down, and a
// LifecycleHook that feeds lifecycle transitions into the supplied recorder.
func newTestEngine(rec *recordingAlertManager) *correlation.Engine {
	return correlation.NewEngine(correlation.Config{
		Window:            10 * time.Minute,
		LifecycleInterval: 1 * time.Minute,
		StartupQuiet:      0,
		ResolveHoldDown:   0,
		Enricher:          &enricher.DefaultEnricher{},
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			rec.NotifyIncident(inc, action)
		},
	})
}

// makeEvent is a shorthand for building an event.Event with commonly-used
// fields set.
func makeEvent(resource, podName, namespace, reason, containerName, nodeName string) event.Event {
	return event.Event{
		Resource:      resource,
		PodName:       podName,
		Namespace:     namespace,
		Reason:        reason,
		ContainerName: containerName,
		NodeName:      nodeName,
		IncludeEvents: true,
		IncludeLogs:   true,
	}
}

// makeContainerState builds a model.ContainerState for use in engine Process calls.
func makeContainerState(restartCount int32, reason string, exitCode int32) *model.ContainerState {
	return &model.ContainerState{
		RestartCount: restartCount,
		Reason:       reason,
		ExitCode:     exitCode,
	}
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

// TestCrashLoopPodCreatesAndResolves verifies that a CrashLoopBackOff event
// opens an incident (ActionCreate) and that explicitly marking it resolved
// produces an ActionResolved notification. Subsequent events for the same
// key are suppressed (edge-triggered ActionSkip) until the state transitions.
func TestCrashLoopPodCreatesAndResolves(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := newTestEngine(rec)

	ev := makeEvent("pod", "my-pod", "default", "CrashLoopBackOff", "main", "node-1")
	owner := "my-deployment"
	cs := makeContainerState(3, "CrashLoopBackOff", 137)

	// First occurrence: incident is created
	inc, action := eng.Process(ev, owner, cs)
	if action != model.ActionCreate {
		t.Fatalf("expected ActionCreate, got %s", action)
	}
	if inc == nil {
		t.Fatal("expected non-nil incident")
	}
	if inc.Key != correlation.BuildKey(ev.Namespace, owner, "CrashLoopBackOff", "") {
		t.Fatalf("unexpected incident key: %s", inc.Key)
	}
	rec.NotifyIncident(inc, action)

	// Second occurrence: edge-triggered → skip (same NotifiedSig)
	inc2, action2 := eng.Process(ev, owner, makeContainerState(4, "CrashLoopBackOff", 137))
	if action2 != model.ActionSkip {
		t.Fatalf("expected ActionSkip (edge-triggered), got %s", action2)
	}
	if inc2 == nil {
		t.Fatal("expected non-nil incident on repeat")
	}

	// Resolve
	eng.MarkResolved(inc.Key)

	// Two notifications: the create we recorded, and the resolved from LifecycleHook
	if rec.Len() != 2 {
		_, a0 := rec.Get(0)
		_, a1 := rec.Get(1)
		t.Fatalf("expected 2 notifications (create + resolved), got %d: [%s, %s]",
			rec.Len(), a0, a1)
	}
	_, createAction := rec.Get(0)
	_, resolveAction := rec.Get(1)
	if createAction != model.ActionCreate {
		t.Fatalf("first notification should be ActionCreate, got %s", createAction)
	}
	if resolveAction != model.ActionResolved {
		t.Fatalf("second notification should be ActionResolved, got %s", resolveAction)
	}
}

// TestNodeConditionCreateAndResolve verifies that a node condition alert
// (e.g. MemoryPressure) creates an incident and that clearing the condition
// resolves it — producing exactly one (create, resolved) pair.
func TestNodeConditionCreateAndResolve(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := newTestEngine(rec)

	ev := makeEvent("node", "worker-1", "", "MemoryPressure", "", "worker-1")
	owner := "worker-1"

	inc, action := eng.Process(ev, owner, nil)
	if action != model.ActionCreate {
		t.Fatalf("expected ActionCreate for node MemoryPressure, got %s", action)
	}
	if inc == nil {
		t.Fatal("expected non-nil node incident")
	}
	rec.NotifyIncident(inc, action)

	// Resolve
	eng.MarkResolved(inc.Key)

	if rec.Len() != 2 {
		t.Fatalf("expected 2 notifications (create + resolved), got %d", rec.Len())
	}

	_, a0 := rec.Get(0)
	if a0 != model.ActionCreate {
		t.Fatalf("expected ActionCreate, got %s", a0)
	}
	_, a1 := rec.Get(1)
	if a1 != model.ActionResolved {
		t.Fatalf("expected ActionResolved, got %s", a1)
	}
}

// TestInhibitionSuppressesPodsDuringNodeFailure verifies that when node
// inhibition is enabled, pod incidents on a node with an active node incident
// are silently suppressed.
func TestInhibitionSuppressesPodsDuringNodeFailure(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := correlation.NewEngine(correlation.Config{
		Window:                    10 * time.Minute,
		LifecycleInterval:         1 * time.Minute,
		StartupQuiet:              0,
		ResolveHoldDown:           0,
		Enricher:                  &enricher.DefaultEnricher{},
		InhibitNodeSuppressesPods: true,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			rec.NotifyIncident(inc, action)
		},
	})

	// Create a node incident on worker-1
	nodeEv := makeEvent("node", "worker-1", "", "NodeNotReady", "", "worker-1")
	nodeInc, nodeAction := eng.Process(nodeEv, "worker-1", nil)
	if nodeAction != model.ActionCreate {
		t.Fatalf("expected ActionCreate for node incident, got %s", nodeAction)
	}
	rec.NotifyIncident(nodeInc, nodeAction)

	// Pod incident on the same node should be suppressed
	podEv := makeEvent("pod", "crashing-pod", "default", "CrashLoopBackOff", "app", "worker-1")
	_, podAction := eng.Process(podEv, "my-deployment", makeContainerState(1, "CrashLoopBackOff", 1))
	if podAction != model.ActionSkip {
		t.Fatalf("expected ActionSkip (node-inhibited), got %s", podAction)
	}

	// Pod incident should NOT appear in notifications
	if rec.Len() != 1 {
		t.Fatalf("expected 1 notification (node create only), got %d", rec.Len())
	}

	// After node resolves, pod should be allowed
	eng.MarkResolved(nodeInc.Key)
	if rec.Len() != 2 {
		t.Fatalf("expected 2 notifications after node resolve, got %d", rec.Len())
	}
}

// TestBaselineSuppressesRestartRepage verifies that a pod whose owner+reason
// was previously seen (seeded via SetSeen) is suppressed on first contact,
// preventing re-paging after restart.
func TestBaselineSuppressesRestartRepage(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := newTestEngine(rec)

	key := correlation.BuildKey("default", "my-deployment", "CrashLoopBackOff", "")

	// Seed a baseline entry so the engine treats the pod as previously seen
	eng.SetSeen(map[string]map[string]int64{
		key: {"my-pod": time.Now().Unix()},
	})

	ev := makeEvent("pod", "my-pod", "default", "CrashLoopBackOff", "main", "")
	inc, action := eng.Process(ev, "my-deployment", makeContainerState(3, "CrashLoopBackOff", 137))
	if action != model.ActionSkip {
		t.Fatalf("expected ActionSkip for baselined pod, got %s", action)
	}
	if inc != nil {
		t.Fatal("expected nil incident for baselined pod")
	}

	// No notifications should be produced
	if rec.Len() != 0 {
		t.Fatalf("expected 0 notifications, got %d", rec.Len())
	}
}

// TestOwnerGroupingSameReason verifies that two pods sharing the same owner
// and reason map to a single incident whose Resources field contains both pod
// names.
func TestOwnerGroupingSameReason(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := newTestEngine(rec)

	owner := "my-deployment"
	cs := makeContainerState(2, "CrashLoopBackOff", 137)

	// First pod
	ev1 := makeEvent("pod", "pod-a", "default", "CrashLoopBackOff", "main", "")
	inc1, action1 := eng.Process(ev1, owner, cs)
	if action1 != model.ActionCreate {
		t.Fatalf("expected ActionCreate for first pod, got %s", action1)
	}
	if inc1 == nil {
		t.Fatal("expected non-nil incident")
	}
	if !inc1.Resources["pod-a"] {
		t.Fatal("first pod should be in Resources")
	}
	rec.NotifyIncident(inc1, action1)

	// Second pod with same owner+reason → same incident key, edge-triggered skip
	ev2 := makeEvent("pod", "pod-b", "default", "CrashLoopBackOff", "main", "")
	inc2, action2 := eng.Process(ev2, owner, makeContainerState(1, "CrashLoopBackOff", 137))
	if action2 != model.ActionSkip {
		t.Fatalf("expected ActionSkip for grouped second pod, got %s", action2)
	}
	if inc2 == nil {
		t.Fatal("expected non-nil incident for grouped pod")
	}

	// Both pods should be in the incident's Resources
	if !inc2.Resources["pod-a"] {
		t.Fatal("pod-a missing from Resources after second Process call")
	}
	if !inc2.Resources["pod-b"] {
		t.Fatal("pod-b missing from Resources")
	}
	if inc2.PeakResources < 2 {
		t.Fatalf("PeakResources should be at least 2, got %d", inc2.PeakResources)
	}

	// Resolve
	eng.MarkResolved(inc1.Key)

	// Expect 2 notifications: the create we recorded + the resolved from hook
	if rec.Len() != 2 {
		t.Fatalf("expected 2 notifications, got %d", rec.Len())
	}
}

// --------------------------------------------------------------------------
// Load tests & benchmarks
// --------------------------------------------------------------------------

// TestStormCollapseUnderLoad verifies that 1000 consecutive events for the
// same (ns,owner,reason) key produce a bounded number of notifications rather
// than generating 1000 separate alerts. Under storm detection, the tenth+
// create within the storm window is absorbed into a digest; edge-triggering
// in steady state also suppresses repeats.
func TestStormCollapseUnderLoad(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := correlation.NewEngine(defaultStormConfig(rec))

	owner := "dep-storm"
	cs := makeContainerState(1, "CrashLoopBackOff", 137)

	n := 1000
	lastAction := model.ActionSkip
	for i := 0; i < n; i++ {
		podName := fmt.Sprintf("pod-%d", i)
		ev := makeEvent("pod", podName, "storm-ns", "CrashLoopBackOff", "main", "")
		_, action := eng.Process(ev, owner, cs)
		if action != model.ActionSkip {
			lastAction = action
			if action == model.ActionCreate || action == model.ActionDigest {
				rec.NotifyIncident(nil, action)
			}
		}
	}

	// Under storm collapse we should never have more than a handful of
	// notifications out of 1000 events. The exact count depends on
	// StormThreshold, but it must be << n.
	if rec.Len() > 50 {
		t.Fatalf("storm collapse expected <= 50 notifications from %d events, got %d (lastAction=%s)",
			n, rec.Len(), lastAction)
	}
}

// TestBoundedStateUnderLoad verifies that the engine's internal state map
// grows only with distinct (ns,owner,reason) keys, not with each event.
func TestBoundedStateUnderLoad(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := correlation.NewEngine(defaultStormConfig(rec))

	// 10 distinct owners × 100 events each = 1000 total events
	distinctOwners := 10
	eventsPerOwner := 100

	cs := makeContainerState(1, "OOMKill", 137)

	for o := 0; o < distinctOwners; o++ {
		owner := fmt.Sprintf("dep-%d", o)
		for i := 0; i < eventsPerOwner; i++ {
			podName := fmt.Sprintf("pod-%d", i)
			ev := makeEvent("pod", podName, "load-ns", "OOMKill", "main", "")
			eng.Process(ev, owner, cs)
		}
	}

	// The engine should hold exactly distinctOwners entries (one per owner)
	// in its active state. We can't access state directly, but we can verify
	// indirectly: resolving each triggers a single notification.
	for o := 0; o < distinctOwners; o++ {
		key := correlation.BuildKey("load-ns", fmt.Sprintf("dep-%d", o), "OOMKill", "")
		eng.MarkResolved(key)
	}

	// DistinctOwners creates + distinctOwners resolves = 2*distinctOwners in
	// best case; storm playbook may add extra. Sanity: << 1000.
	if rec.Len() > 100 {
		t.Fatalf("expected bounded notifications <= 100, got %d", rec.Len())
	}
}

// BenchmarkProcessStorm measures allocation and throughput of engine.Process
// under a bulk-load scenario simulating storm collapse.
func BenchmarkProcessStorm(b *testing.B) {
	rec := &recordingAlertManager{}
	eng := correlation.NewEngine(defaultStormConfig(rec))

	owner := "dep-bench"
	cs := makeContainerState(1, "CrashLoopBackOff", 137)

	ev := makeEvent("pod", "pod", "bench-ns", "CrashLoopBackOff", "main", "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eng.Process(ev, owner, cs)
	}
}
