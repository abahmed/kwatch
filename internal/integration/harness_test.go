package integration

import (
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// defaultConfig returns a basic Config suitable for integration tests.
func defaultConfig(rec *recordingAlertManager) correlation.Config {
	return correlation.Config{
		Window:            10 * time.Minute,
		LifecycleInterval: 1 * time.Minute,
		ResolveHoldDown:   0,
		Enricher:          &enricher.DefaultEnricher{},
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
// integration tests. It stands in for the real alert.AlertManager, wired
// through the correlation engine's LifecycleHook — the engine announces every
// decision itself, including the ones Process returns, so tests must not
// record the return value a second time.
type recordingAlertManager struct {
	mu       sync.Mutex
	notified []alertEntry
}

func (r *recordingAlertManager) NotifyIncident(
	inc *model.Incident,
	action model.IncidentAction,
	_ ...interface{},
) {
	r.mu.Lock()
	r.notified = append(r.notified, alertEntry{inc: inc, action: action})
	r.mu.Unlock()
}

func (r *recordingAlertManager) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.notified)
}

func (r *recordingAlertManager) Get(
	i int,
) (*model.Incident, model.IncidentAction) {
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
		ResolveHoldDown:   0,
		Enricher:          &enricher.DefaultEnricher{},
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			rec.NotifyIncident(inc, action)
		},
	})
}

// makeEvent is a shorthand for building an event.Event with commonly-used
// fields set.
func makeEvent(
	resource, podName, namespace, reason, containerName, nodeName string,
) event.Event {
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

// makeContainerState builds a model.ContainerState for use in engine Process
// calls.
func makeContainerState(
	restartCount int32,
	reason string,
	exitCode int32,
) *model.ContainerState {
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

	ev := makeEvent(
		"pod",
		"my-pod",
		"default",
		"CrashLoopBackOff",
		"main",
		"node-1",
	)
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
	if inc.Key != correlation.BuildKey(
		ev.Namespace,
		owner,
		"CrashLoopBackOff",
		"",
	) {
		t.Fatalf("unexpected incident key: %s", inc.Key)
	}

	// Second occurrence: edge-triggered → skip (same NotifiedSig)
	inc2, action2 := eng.Process(
		ev,
		owner,
		makeContainerState(4, "CrashLoopBackOff", 137),
	)
	if action2 != model.ActionSkip {
		t.Fatalf("expected ActionSkip (edge-triggered), got %s", action2)
	}
	if inc2 == nil {
		t.Fatal("expected non-nil incident on repeat")
	}

	// Resolve
	eng.MarkResolved(inc.Key)

	// Two notifications: the create we recorded, and the resolved from
	// LifecycleHook
	if rec.Len() != 2 {
		_, a0 := rec.Get(0)
		_, a1 := rec.Get(1)
		t.Fatalf(
			"expected 2 notifications (create + resolved), got %d: [%s, %s]",
			rec.Len(),
			a0,
			a1,
		)
	}
	_, createAction := rec.Get(0)
	_, resolveAction := rec.Get(1)
	if createAction != model.ActionCreate {
		t.Fatalf(
			"first notification should be ActionCreate, got %s",
			createAction,
		)
	}
	if resolveAction != model.ActionResolved {
		t.Fatalf(
			"second notification should be ActionResolved, got %s",
			resolveAction,
		)
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
		t.Fatalf(
			"expected ActionCreate for node MemoryPressure, got %s",
			action,
		)
	}
	if inc == nil {
		t.Fatal("expected non-nil node incident")
	}

	// Resolve
	eng.MarkResolved(inc.Key)

	if rec.Len() != 2 {
		t.Fatalf(
			"expected 2 notifications (create + resolved), got %d",
			rec.Len(),
		)
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

	// Pod incident on the same node should be suppressed
	podEv := makeEvent(
		"pod",
		"crashing-pod",
		"default",
		"CrashLoopBackOff",
		"app",
		"worker-1",
	)
	_, podAction := eng.Process(
		podEv,
		"my-deployment",
		makeContainerState(1, "CrashLoopBackOff", 1),
	)
	if podAction != model.ActionSkip {
		t.Fatalf("expected ActionSkip (node-inhibited), got %s", podAction)
	}

	// Pod incident should NOT appear in notifications
	if rec.Len() != 1 {
		t.Fatalf(
			"expected 1 notification (node create only), got %d",
			rec.Len(),
		)
	}

	// After node resolves, pod should be allowed
	eng.MarkResolved(nodeInc.Key)
	if rec.Len() != 2 {
		t.Fatalf(
			"expected 2 notifications after node resolve, got %d",
			rec.Len(),
		)
	}
}

// TestBaselineSuppressesRestartRepage verifies that a pod whose owner+reason
// was previously seen (seeded via SetBaseline) is suppressed on first contact,
// preventing re-paging after restart.
func TestBaselineSuppressesRestartRepage(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := newTestEngine(rec)

	key := correlation.BuildKey(
		"default",
		"my-deployment",
		"CrashLoopBackOff",
		"",
	)

	// Seed a baseline entry so the engine treats the pod as previously seen
	eng.SetBaseline(map[string]map[string]int64{
		string(key): {"my-pod": time.Now().Unix()},
	})

	ev := makeEvent("pod", "my-pod", "default", "CrashLoopBackOff", "main", "")
	inc, action := eng.Process(
		ev,
		"my-deployment",
		makeContainerState(3, "CrashLoopBackOff", 137),
	)
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
