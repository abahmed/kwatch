package knowledge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRelatedEdgesBothDirections(t *testing.T) {
	m := NewModel(Options{})
	podID := NewEntityID("pod", "default", "my-pod")
	nodeID := NewEntityID("node", "", "node-1")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	fact := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: RunsOn,
		Targets:  []EntityID{nodeID},
	}
	_, _ = m.Apply(fact)

	// Outgoing: pod -> node
	outgoing := m.Related(podID, RunsOn, Outgoing)
	assert.Equal(t, []EntityID{nodeID}, outgoing)

	// Incoming: node <- pod
	incoming := m.Related(nodeID, RunsOn, Incoming)
	assert.Equal(t, []EntityID{podID}, incoming)
}

func TestRelatedReplaceTargets(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	oldID := NewEntityID("pvc", "default", "old-pvc")
	newID := NewEntityID("pvc", "default", "new-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Initial relation to old target
	fact1 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{oldID},
	}
	_, _ = m.Apply(fact1)

	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{oldID}, targets)

	// Replace with new target
	fact2 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{newID},
	}
	_, _ = m.Apply(fact2)

	targets = m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{newID}, targets)

	// Old target should no longer see incoming edge
	incoming := m.Related(oldID, Mounts, Incoming)
	assert.Empty(t, incoming)

	// New target should see incoming edge
	incoming = m.Related(newID, Mounts, Incoming)
	assert.Equal(t, []EntityID{fromID}, incoming)
}

func TestRelatedTouched(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	target1 := NewEntityID("pvc", "default", "pvc-1")
	target2 := NewEntityID("pvc", "default", "pvc-2")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// First relation
	fact1 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{target1},
	}
	update1, _ := m.Apply(fact1)
	assert.Equal(t, []EntityID{fromID, target1}, update1.Touched)

	// Replace target1 with target2
	fact2 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{target2},
	}
	update2, _ := m.Apply(fact2)
	// Touched should include entity, old target, and new target
	assert.Contains(t, update2.Touched, fromID)
	assert.Contains(t, update2.Touched, target1)
	assert.Contains(t, update2.Touched, target2)
	assert.Equal(t, 3, len(update2.Touched))
}

func TestRelatedMultipleTargets(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("deployment", "default", "my-app")
	targets := []EntityID{
		NewEntityID("pod", "default", "pod-1"),
		NewEntityID("pod", "default", "pod-2"),
		NewEntityID("pod", "default", "pod-3"),
	}
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	fact := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: OwnedBy,
		Targets:  targets,
	}
	_, _ = m.Apply(fact)

	result := m.Related(fromID, OwnedBy, Outgoing)
	assert.Equal(t, len(targets), len(result))
	// Verify all targets are present (sorted)
	for _, target := range targets {
		assert.Contains(t, result, target)
	}
}

func TestRelatedMultiSourceSameEdge(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	targetID := NewEntityID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Source 1 declares edge
	fact1 := Fact{
		Kind:     Related,
		Source:   "source1",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{targetID},
	}
	_, _ = m.Apply(fact1)

	// Source 2 also declares same edge
	fact2 := Fact{
		Kind:     Related,
		Source:   "source2",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{targetID},
	}
	_, _ = m.Apply(fact2)

	// Edge should still be present
	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{targetID}, targets)

	// Withdraw from source 1 - edge should stay (source 2 still declares it)
	fact3 := Fact{
		Kind:     Related,
		Source:   "source1",
		At:       now.Add(2 * time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{},
	}
	_, _ = m.Apply(fact3)

	targets = m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{targetID}, targets)

	// Withdraw from source 2 - edge should disappear
	fact4 := Fact{
		Kind:     Related,
		Source:   "source2",
		At:       now.Add(3 * time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{},
	}
	_, _ = m.Apply(fact4)

	targets = m.Related(fromID, Mounts, Outgoing)
	assert.Empty(t, targets)
}

func TestRelatedDuplicateTargets(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	targetID := NewEntityID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Apply fact with duplicate targets
	fact := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets: []EntityID{
			targetID, targetID, targetID,
		},
	}
	_, _ = m.Apply(fact)

	// Should deduplicate
	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{targetID}, targets)
}

func TestRelatedZeroIDTargetsIgnored(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	validID1 := NewEntityID("pvc", "default", "pvc-1")
	validID2 := NewEntityID("pvc", "default", "pvc-2")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Apply fact with mix of valid IDs and zero IDs
	fact := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets: []EntityID{
			validID1,
			EntityID{}, // Zero ID (Kind="" and Name="")
			validID2,
			EntityID{}, // Another zero ID
		},
	}
	_, _ = m.Apply(fact)

	// Only valid IDs should be present
	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, 2, len(targets))
	assert.Contains(t, targets, validID1)
	assert.Contains(t, targets, validID2)
}

func TestRelatedEmptyTargetsClears(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	target1 := NewEntityID("pvc", "default", "pvc-1")
	target2 := NewEntityID("pvc", "default", "pvc-2")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add two targets
	fact1 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{target1, target2},
	}
	_, _ = m.Apply(fact1)

	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, 2, len(targets))

	// Empty targets list removes all
	fact2 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{},
	}
	_, _ = m.Apply(fact2)

	targets = m.Related(fromID, Mounts, Outgoing)
	assert.Empty(t, targets)

	// Old targets no longer have incoming edges
	incoming := m.Related(target1, Mounts, Incoming)
	assert.Empty(t, incoming)
}

func TestRelationsListingAllTypes(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	node := NewEntityID("node", "", "node-1")
	pvc := NewEntityID("pvc", "default", "my-pvc")
	cm := NewEntityID("configmap", "default", "my-config")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add different relation types
	facts := []Fact{
		{Kind: Related, Source: "k8s", At: now, Entity: fromID,
			Relation: RunsOn, Targets: []EntityID{node}},
		{Kind: Related, Source: "k8s", At: now, Entity: fromID,
			Relation: Mounts, Targets: []EntityID{pvc}},
		{Kind: Related, Source: "k8s", At: now, Entity: fromID,
			Relation: References, Targets: []EntityID{cm}},
	}

	for _, fact := range facts {
		_, _ = m.Apply(fact)
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
	fromID := NewEntityID("pod", "default", "my-pod")
	targets := []EntityID{
		NewEntityID("pvc", "default", "z-pvc"),
		NewEntityID("pvc", "default", "a-pvc"),
		NewEntityID("pvc", "default", "m-pvc"),
	}
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add all targets in one fact, in non-sorted order
	fact := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  targets,
	}
	_, _ = m.Apply(fact)

	// Query multiple times - should get same sorted order
	result1 := m.Related(fromID, Mounts, Outgoing)
	result2 := m.Related(fromID, Mounts, Outgoing)

	assert.Equal(t, result1, result2)
	assert.Equal(t, 3, len(result1))
	assert.Equal(t, "a-pvc", result1[0].Name)
	assert.Equal(t, "m-pvc", result1[1].Name)
	assert.Equal(t, "z-pvc", result1[2].Name)
}

func TestRelatedInvalidRelation(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	fact := Fact{
		Kind:   Related,
		Source: "k8s",
		At:     time.Now(),
		Entity: id,
		// Relation is empty/zero
		Targets: []EntityID{},
	}
	_, err := m.Apply(fact)
	assert.Equal(t, ErrInvalidRelation, err)
}

func TestRelatedStatsCounts(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add 3 pods with relations to 2 nodes
	pods := []EntityID{
		NewEntityID("pod", "default", "pod-1"),
		NewEntityID("pod", "default", "pod-2"),
		NewEntityID("pod", "default", "pod-3"),
	}
	nodes := []EntityID{
		NewEntityID("node", "", "node-1"),
		NewEntityID("node", "", "node-2"),
	}

	// Each pod has a relation to both nodes via single fact
	for _, pod := range pods {
		fact := Fact{
			Kind:     Related,
			Source:   "k8s",
			At:       now,
			Entity:   pod,
			Relation: RunsOn,
			Targets:  nodes, // Both nodes at once
		}
		_, _ = m.Apply(fact)
	}

	stats := m.Stats()
	// 3 pods * 2 nodes = 6 relations
	assert.Equal(t, 6, stats.Relations)
}
