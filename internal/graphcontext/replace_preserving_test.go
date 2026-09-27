package graphcontext

import "testing"

func TestReplaceWithPreservingKeepsUnbuiltKindEdges(t *testing.T) {
	old := NewResourceGraph()
	old.AddEdge("pod", "ns", "p1", "node", "", "worker-1", "scheduled_on")
	old.AddEdge(
		"gateway", "ns", "gw1", "secret", "ns", "tls", "references",
	)

	next := NewResourceGraph()
	next.AddEdge("pod", "ns", "p1", "node", "", "worker-2", "scheduled_on")

	old.ReplaceWithPreserving(next)

	dependents := old.DependentsOf("secret", "ns", "tls")
	found := false
	for _, dep := range dependents {
		if dep == resourceKey("gateway", "ns", "gw1") {
			found = true
		}
	}
	if !found {
		t.Fatalf(
			"expected preserved gateway->secret edge, dependents: %+v",
			dependents,
		)
	}

	deps := old.DependenciesOf("pod", "ns", "p1")
	for _, dep := range deps {
		if dep == resourceKey("node", "", "worker-1") {
			t.Fatalf("expected stale pod->node edge gone, deps: %+v", deps)
		}
	}
	foundNewNode := false
	for _, dep := range deps {
		if dep == resourceKey("node", "", "worker-2") {
			foundNewNode = true
		}
	}
	if !foundNewNode {
		t.Fatalf("expected new pod->node edge present, deps: %+v", deps)
	}
}
