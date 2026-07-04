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
