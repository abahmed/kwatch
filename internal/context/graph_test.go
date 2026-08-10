package context

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResourceGraph(t *testing.T) {
	g := NewResourceGraph()
	assert.NotNil(t, g)
	assert.Empty(t, g.Edges())
}

func TestAddEdge(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")

	deps := g.DependenciesOf("pod", "ns1", "p1")
	assert.Len(t, deps, 1)
	assert.Equal(t, "node//n1", deps[0])

	deps2 := g.DependentsOf("node", "", "n1")
	assert.Len(t, deps2, 1)
	assert.Equal(t, "pod/ns1/p1", deps2[0])
}

func TestAddSelfEdgeIgnored(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "pod", "ns1", "p1", "self")
	assert.Empty(t, g.Edges())
}

func TestDependentsByType(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	g.AddEdge("pod", "ns1", "p2", "node", "", "n1", "scheduled_on")
	g.AddEdge("pod", "ns1", "p3", "node", "", "n2", "scheduled_on")

	pods := g.DependentsByType("node", "", "n1", "pod")
	assert.Len(t, pods, 2)
	assert.Contains(t, pods, "pod/ns1/p1")
	assert.Contains(t, pods, "pod/ns1/p2")

	empty := g.DependentsByType("node", "", "n1", "service")
	assert.Empty(t, empty)
}

func TestRemoveNode(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	g.AddEdge("pod", "ns1", "p2", "node", "", "n1", "scheduled_on")

	g.RemoveNode("pod", "ns1", "p1")

	assert.Empty(t, g.DependenciesOf("pod", "ns1", "p1"))
	deps := g.DependentsOf("node", "", "n1")
	assert.Len(t, deps, 1)
	assert.Equal(t, "pod/ns1/p2", deps[0])
}

func TestRemoveNodeWithDependents(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")

	g.RemoveNode("node", "", "n1")

	assert.Empty(t, g.DependenciesOf("pod", "ns1", "p1"))
	assert.Empty(t, g.DependentsOf("node", "", "n1"))
}

func TestClear(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	g.Clear()

	assert.Empty(t, g.DependenciesOf("pod", "ns1", "p1"))
	assert.Empty(t, g.DependentsOf("node", "", "n1"))
	assert.Empty(t, g.Edges())
}

func TestEdgeList(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")

	edges := g.Edges()
	assert.Len(t, edges, 1)
	assert.Equal(t, "pod/ns1/p1", edges[0].From)
	assert.Equal(t, "node//n1", edges[0].To)
	assert.Equal(t, "scheduled_on", edges[0].Type)
}

func TestMultipleEdges(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	g.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")

	deps := g.DependenciesOf("pod", "ns1", "p1")
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "node//n1")
	assert.Contains(t, deps, "deployment/ns1/dep1")
}

func TestEdgesDoNotGrowOnRepeatedAdd(t *testing.T) {
	g := NewResourceGraph()
	for i := 0; i < 1000; i++ {
		g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
		g.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")
	}
	// Repeated updates (pod relists/rebuilds) must not accumulate edges.
	assert.Len(t, g.Edges(), 2)
	assert.Len(t, g.DependenciesOf("pod", "ns1", "p1"), 2)
}

func TestRemoveNodeCleansEdges(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	g.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")

	g.RemoveNode("pod", "ns1", "p1")

	assert.Empty(t, g.Edges())
	assert.Empty(t, g.DependentsOf("node", "", "n1"))
	assert.Empty(t, g.DependentsOf("deployment", "ns1", "dep1"))
}

func TestRemoveEdgesFrom(t *testing.T) {
	g := NewResourceGraph()
	g.AddEdge("ingress", "ns1", "ing1", "service", "ns1", "svc1", "routes_to")
	g.AddEdge("ingress", "ns1", "ing1", "service", "ns1", "svc2", "routes_to")
	// Incoming edge from another node must be preserved.
	g.AddEdge("deployment", "ns1", "dep1", "ingress", "ns1", "ing1", "exposes")

	g.RemoveEdgesFrom("ingress", "ns1", "ing1", "routes_to")

	assert.Empty(t, g.DependenciesOf("ingress", "ns1", "ing1"))
	assert.Empty(t, g.DependentsOf("service", "ns1", "svc1"))
	assert.Empty(t, g.DependentsOf("service", "ns1", "svc2"))
	// Node itself and other node's edge remain.
	assert.Contains(t, g.DependentsOf("ingress", "ns1", "ing1"), "deployment/ns1/dep1")
}

func TestTraverseDependentsTransitive(t *testing.T) {
	g := NewResourceGraph()
	// node ← pod ← service ← ingress
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	g.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")
	g.AddEdge("ingress", "ns1", "ing1", "service", "ns1", "svc1", "routes_to")

	reached := g.TraverseDependents("node", "", "n1")
	assert.ElementsMatch(t, []string{
		"pod/ns1/p1",
		"service/ns1/svc1",
		"ingress/ns1/ing1",
	}, reached)
}

func TestTraverseDependentsStopsAtDiamond(t *testing.T) {
	g := NewResourceGraph()
	// node ← p1, node ← p2, svc ← p1, svc ← p2 — svc must appear once.
	g.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	g.AddEdge("pod", "ns1", "p2", "node", "", "n1", "scheduled_on")
	g.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")
	g.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p2", "selects")

	reached := g.TraverseDependents("node", "", "n1")
	assert.Equal(t, 3, len(reached)) // both pods + svc once
	assert.Contains(t, reached, "service/ns1/svc1")
}

func TestConcurrency(t *testing.T) {
	g := NewResourceGraph()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			g.AddEdge("pod", "ns", "p", "node", "", "n", "type")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 50; i++ {
			g.DependenciesOf("pod", "ns", "p")
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

func TestTraverseDependenciesTransitive(t *testing.T) {
	g := NewResourceGraph()
	// service ← p1 ← configmap; p1 also ← node
	g.AddEdge("pod", "ns", "p1", "service", "ns", "svc", "selects")
	g.AddEdge("pod", "ns", "p1", "configmap", "ns", "cm", "mounts")
	g.AddEdge("configmap", "ns", "cm", "node", "", "n1", "scheduled_on")

	// From pod we reach every resource it directly or transitively depends on.
	deps := g.TraverseDependencies("pod", "ns", "p1")
	assert.ElementsMatch(t, []string{
		"service/ns/svc",
		"configmap/ns/cm",
		"node//n1", // via configmap
	}, deps)

	// From service, nothing upstream.
	assert.Empty(t, g.TraverseDependencies("service", "ns", "svc"))
}
