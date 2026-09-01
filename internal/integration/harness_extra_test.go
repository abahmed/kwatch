package integration

import (
	"testing"

	"github.com/abahmed/kwatch/internal/model"
)

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

	// Second pod with same owner+reason → same incident key, edge-triggered
	// skip
	ev2 := makeEvent("pod", "pod-b", "default", "CrashLoopBackOff", "main", "")
	inc2, action2 := eng.Process(
		ev2,
		owner,
		makeContainerState(1, "CrashLoopBackOff", 137),
	)
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
		t.Fatalf(
			"PeakResources should be at least 2, got %d",
			inc2.PeakResources,
		)
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

// TestBoundedStateUnderLoad verifies that the engine's internal state map
// grows only with distinct (ns,owner,reason) keys, not with each event.
