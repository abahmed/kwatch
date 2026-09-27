package kubeletmetrics

import (
	"testing"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

// TestObserveOwnedWaitsForWholeGroupToRecover verifies that a group resolve
// callback only runs once every key in the group has recovered, not as soon
// as the first sibling does.
func TestObserveOwnedWaitsForWholeGroupToRecover(t *testing.T) {
	client := fake.NewSimpleClientset()
	m := newTestMonitor(client, config.KubeletTelemetryMonitor{}, nil)

	var resolvedA, resolvedB int
	reportNoop := func() {}
	resolveA := func() { resolvedA++ }
	resolveB := func() { resolvedB++ }

	// Both keys in group "g" start failing.
	m.observeOwned("a", "g", true, reportNoop, resolveA)
	m.observeOwned("b", "g", true, reportNoop, resolveB)

	// "a" recovers first; "b" is still failing, so its resolve must not run.
	m.observeOwned("a", "g", false, reportNoop, resolveA)
	if resolvedA != 0 {
		t.Fatalf("resolve ran for %q while %q still failing", "a", "b")
	}

	// "b" now recovers too; both group members are healthy, so it resolves.
	m.observeOwned("b", "g", false, reportNoop, resolveB)
	if resolvedB != 1 {
		t.Fatalf("expected resolve for %q once group cleared, got %d",
			"b", resolvedB)
	}
}
