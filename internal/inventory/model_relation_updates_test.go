package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRelatedReplaceTargets(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("pod", "default", "my-pod")
	oldID := CoreID("pvc", "default", "old-pvc")
	newID := CoreID("pvc", "default", "new-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Initial relation to old target
	observation1 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{oldID},
	}
	_, _ = m.Apply(observation1)

	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{oldID}, targets)

	// Replace with new target
	observation2 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{newID},
	}
	_, _ = m.Apply(observation2)

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
	fromID := CoreID("pod", "default", "my-pod")
	target1 := CoreID("pvc", "default", "pvc-1")
	target2 := CoreID("pvc", "default", "pvc-2")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// First relation
	observation1 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{target1},
	}
	update1, _ := m.Apply(observation1)
	assert.Equal(t, []EntityID{fromID, target1}, update1.Touched)

	// Replace target1 with target2
	observation2 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{target2},
	}
	update2, _ := m.Apply(observation2)
	// Touched should include entity, old target, and new target
	assert.Contains(t, update2.Touched, fromID)
	assert.Contains(t, update2.Touched, target1)
	assert.Contains(t, update2.Touched, target2)
	assert.Equal(t, 3, len(update2.Touched))
}

func TestRelatedDuplicateTargets(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("pod", "default", "my-pod")
	targetID := CoreID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Apply observation with duplicate targets
	observation := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets: []EntityID{
			targetID, targetID, targetID,
		},
	}
	_, _ = m.Apply(observation)

	// Should deduplicate
	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{targetID}, targets)
}

func TestRelatedZeroIDTargetsIgnored(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("pod", "default", "my-pod")
	validID1 := CoreID("pvc", "default", "pvc-1")
	validID2 := CoreID("pvc", "default", "pvc-2")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Apply observation with mix of valid IDs and zero IDs
	observation := Observation{
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
	_, _ = m.Apply(observation)

	// Only valid IDs should be present
	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, 2, len(targets))
	assert.Contains(t, targets, validID1)
	assert.Contains(t, targets, validID2)
}

func TestRelatedEmptyTargetsClears(t *testing.T) {
	m := NewModel(Options{})
	fromID := CoreID("pod", "default", "my-pod")
	target1 := CoreID("pvc", "default", "pvc-1")
	target2 := CoreID("pvc", "default", "pvc-2")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add two targets
	observation1 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{target1, target2},
	}
	_, _ = m.Apply(observation1)

	targets := m.Related(fromID, Mounts, Outgoing)
	assert.Equal(t, 2, len(targets))

	// Empty targets list removes all
	observation2 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now.Add(time.Second),
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{},
	}
	_, _ = m.Apply(observation2)

	targets = m.Related(fromID, Mounts, Outgoing)
	assert.Empty(t, targets)

	// Old targets no longer have incoming edges
	incoming := m.Related(target1, Mounts, Incoming)
	assert.Empty(t, incoming)
}

func TestRelatedInvalidRelation(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	observation := Observation{
		Kind:   Related,
		Source: "k8s",
		At:     time.Now(),
		Entity: id,
		// Relation is empty/zero
		Targets: []EntityID{},
	}
	_, err := m.Apply(observation)
	assert.Equal(t, ErrInvalidRelation, err)
}
