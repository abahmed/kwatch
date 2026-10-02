package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRelatedEdgesBothDirections(t *testing.T) {
	m := NewModel(Options{})
	podID := CoreID("pod", "default", "my-pod")
	nodeID := CoreID("node", "", "node-1")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	observation := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: RunsOn,
		Targets:  []EntityID{nodeID},
	}
	_, _ = m.Apply(observation)

	// Outgoing: pod -> node
	outgoing := m.Related(podID, RunsOn, Outgoing)
	assert.Equal(t, []EntityID{nodeID}, outgoing)

	// Incoming: node <- pod
	incoming := m.Related(nodeID, RunsOn, Incoming)
	assert.Equal(t, []EntityID{podID}, incoming)
}

func TestRelatedMultipleTargets(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("deployment", "default", "my-app")
	targets := []EntityID{
		CoreID("pod", "default", "pod-1"),
		CoreID("pod", "default", "pod-2"),
		CoreID("pod", "default", "pod-3"),
	}
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	observation := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: OwnedBy,
		Targets:  targets,
	}
	_, _ = m.Apply(observation)

	result := m.Related(fromID, OwnedBy, Outgoing)
	assert.Equal(t, len(targets), len(result))
	// Verify all targets are present (sorted)
	for _, target := range targets {
		assert.Contains(t, result, target)
	}
}

func TestRelatedMultiSourceSameEdge(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("pod", "default", "my-pod")
	targetID := CoreID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Source 1 declares edge
	observation1 := Observation{
		Kind:     Related,
		Source:   "source1",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{targetID},
	}
	_, _ = m.Apply(observation1)

	// Source 2 also declares same edge
	observation2 := Observation{
		Kind:     Related,
		Source:   "source2",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{targetID},
	}
	_, _ = m.Apply(observation2)

	// Edge should still be present
	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{targetID}, targets)

	// Withdraw from source 1 - edge should stay (source 2 still declares it)
	observation3 := Observation{
		Kind:     Related,
		Source:   "source1",
		At:       now.Add(2 * time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{},
	}
	_, _ = m.Apply(observation3)

	targets = m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{targetID}, targets)

	// Withdraw from source 2 - edge should disappear
	observation4 := Observation{
		Kind:     Related,
		Source:   "source2",
		At:       now.Add(3 * time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{},
	}
	_, _ = m.Apply(observation4)

	targets = m.Related(fromID, Mounts, Outgoing)
	assert.Empty(t, targets)
}

func TestRelationsListingAllTypes(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("pod", "default", "my-pod")
	node := CoreID("node", "", "node-1")
	pvc := CoreID("pvc", "default", "my-pvc")
	cm := CoreID("configmap", "default", "my-config")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add different relation types
	observations := []Observation{
		{Kind: Related, Source: "k8s", At: now, Entity: fromID,
			Relation: RunsOn, Targets: []EntityID{node}},
		{Kind: Related, Source: "k8s", At: now, Entity: fromID,
			Relation: Mounts, Targets: []EntityID{pvc}},
		{Kind: Related, Source: "k8s", At: now, Entity: fromID,
			Relation: References, Targets: []EntityID{cm}},
	}

	for _, observation := range observations {
		_, _ = m.Apply(observation)
	}

	relations := m.Relations(fromID, Outgoing)
	assert.Equal(t, 3, len(relations))

	// Verify all relation types are present
	types := make(map[RelationType]bool)
	for _, rel := range relations {
		types[rel.Type] = true
	}
	assert.True(t, types[RunsOn])
	assert.True(t, types[Mounts])
	assert.True(t, types[References])
}

func TestRelationsDeterministicOrder(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("pod", "default", "my-pod")
	targets := []EntityID{
		CoreID("pvc", "default", "z-pvc"),
		CoreID("pvc", "default", "a-pvc"),
		CoreID("pvc", "default", "m-pvc"),
	}
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add all targets in one observation, in non-sorted order
	observation := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  targets,
	}
	_, _ = m.Apply(observation)

	// Query multiple times - should get same sorted order
	result1 := m.Related(fromID, Mounts, Outgoing)
	result2 := m.Related(fromID, Mounts, Outgoing)

	assert.Equal(t, result1, result2)
	assert.Equal(t, 3, len(result1))
	assert.Equal(t, "a-pvc", result1[0].Name)
	assert.Equal(t, "m-pvc", result1[1].Name)
	assert.Equal(t, "z-pvc", result1[2].Name)
}

func TestRelatedStatsCounts(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add 3 pods with relations to 2 nodes
	pods := []EntityID{
		CoreID("pod", "default", "pod-1"),
		CoreID("pod", "default", "pod-2"),
		CoreID("pod", "default", "pod-3"),
	}
	nodes := []EntityID{
		CoreID("node", "", "node-1"),
		CoreID("node", "", "node-2"),
	}

	// Each pod has a relation to both nodes via single observation
	for _, pod := range pods {
		observation := Observation{
			Kind:     Related,
			Source:   "k8s",
			At:       now,
			Entity:   pod,
			Relation: RunsOn,
			Targets:  nodes, // Both nodes at once
		}
		_, _ = m.Apply(observation)
	}

	stats := m.Stats()
	// 3 pods * 2 nodes = 6 relations
	assert.Equal(t, 6, stats.Relations)
}
