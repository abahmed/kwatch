package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestChangedAppended(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	change1 := Change{
		At:     now,
		Actor:  "user1",
		Fields: []FieldChange{{Path: "spec.replicas", Before: "1", After: "2"}},
	}

	observation1 := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change1,
	}
	_, _ = m.Apply(observation1)

	changes := m.Changes(id, time.Time{})
	assert.Equal(t, 1, len(changes))
	assert.Equal(t, now, changes[0].At)
	assert.Equal(t, "user1", changes[0].Actor)

	// Add another change
	change2 := Change{
		At:     now.Add(time.Second),
		Actor:  "user2",
		Fields: []FieldChange{{Path: "spec.image", Before: "v1", After: "v2"}},
	}

	observation2 := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
		Change: change2,
	}
	_, _ = m.Apply(observation2)

	changes = m.Changes(id, time.Time{})
	assert.Equal(t, 2, len(changes))
	assert.Equal(t, "user1", changes[0].Actor)
	assert.Equal(t, "user2", changes[1].Actor)
}

func TestChangedAtDefaultsToObservationAt(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Change with zero At
	change := Change{
		Actor: "user1",
	}

	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	changes := m.Changes(id, time.Time{})
	assert.Equal(t, 1, len(changes))
	// At should default to observation.At
	assert.Equal(t, now, changes[0].At)
}

func TestChangedMaxChangesPerEntity(t *testing.T) {
	m := NewModel(Options{MaxChangesPerEntity: 5})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add 10 changes
	for i := 0; i < 10; i++ {
		change := Change{
			At:    now.Add(time.Duration(i) * time.Second),
			Actor: "user",
		}
		observation := Observation{
			Kind:   Changed,
			Source: "k8s",
			At:     now.Add(time.Duration(i) * time.Second),
			Entity: id,
			Change: change,
		}
		_, _ = m.Apply(observation)
	}

	changes := m.Changes(id, time.Time{})
	// Only last 5 should be kept
	assert.Equal(t, 5, len(changes))
	// Oldest should be the 6th change (index 5 in original order, at 5 seconds)
	assert.Equal(t, now.Add(5*time.Second), changes[0].At)
	assert.Equal(t, now.Add(9*time.Second), changes[4].At)
}

func TestChangedSinceFiltering(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t1.Add(2 * time.Hour)

	// Add changes at different times
	for _, ts := range []time.Time{t1, t2, t3} {
		change := Change{At: ts, Actor: "user"}
		observation := Observation{
			Kind:   Changed,
			Source: "k8s",
			At:     ts,
			Entity: id,
			Change: change,
		}
		_, _ = m.Apply(observation)
	}

	// Query since t2 - should include t2 and t3
	changes := m.Changes(id, t2)
	assert.Equal(t, 2, len(changes))
	assert.Equal(t, t2, changes[0].At)
	assert.Equal(t, t3, changes[1].At)

	// Query since after t3 - should be empty
	changes = m.Changes(id, t3.Add(time.Second))
	assert.Empty(t, changes)
}

func TestChangedFieldsDetached(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	change := Change{
		At:    now,
		Actor: "user",
		Fields: []FieldChange{
			{Path: "spec.image", Before: "v1", After: "v2"},
		},
	}

	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	changes1 := m.Changes(id, time.Time{})
	fields1 := changes1[0].Fields

	// Mutate the returned Fields
	fields1[0].Path = "mutated"

	// Get changes again and verify original is unchanged
	changes2 := m.Changes(id, time.Time{})
	assert.Equal(t, 1, len(changes2[0].Fields))
	assert.Equal(t, "spec.image", changes2[0].Fields[0].Path)
}

func TestChangedEntityFilled(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Apply change with Entity field empty (should be filled)
	change := Change{
		At:    now,
		Actor: "user",
	}

	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	changes := m.Changes(id, time.Time{})
	assert.Equal(t, id, changes[0].Entity)
}

func TestChangedCreatedFlag(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "new-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	change := Change{
		At:      now,
		Created: true,
		Actor:   "system",
	}

	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	changes := m.Changes(id, time.Time{})
	assert.True(t, changes[0].Created)
	assert.False(t, changes[0].Deleted)
}

func TestChangedDeletedFlag(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	change := Change{
		At:      now,
		Deleted: true,
		Actor:   "system",
	}

	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	changes := m.Changes(id, time.Time{})
	assert.False(t, changes[0].Created)
	assert.True(t, changes[0].Deleted)
}

func TestChangedMultipleFields(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	change := Change{
		At:    now,
		Actor: "user",
		Fields: []FieldChange{
			{Path: "spec.image", Before: "v1", After: "v2"},
			{Path: "spec.cpu", Before: "100m", After: "200m"},
			{Path: "spec.memory", Before: "128Mi", After: "256Mi"},
		},
	}

	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	changes := m.Changes(id, time.Time{})
	assert.Equal(t, 3, len(changes[0].Fields))
	assert.Equal(t, "spec.image", changes[0].Fields[0].Path)
	assert.Equal(t, "spec.cpu", changes[0].Fields[1].Path)
	assert.Equal(t, "spec.memory", changes[0].Fields[2].Path)
}

func TestChangedInvalidEntity(t *testing.T) {
	m := NewModel(Options{})
	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     time.Now(),
		Entity: EntityID{}, // Zero entity
		Change: Change{},
	}
	_, err := m.Apply(observation)
	assert.Equal(t, ErrInvalidEntity, err)
}

func TestChangedUnknownEntity(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "unknown-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Apply change to entity that was never observed
	change := Change{At: now, Actor: "user"}
	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	// Entity still doesn't exist
	_, exists := m.Entity(id)
	assert.False(t, exists)

	// But change is recorded
	changes := m.Changes(id, time.Time{})
	assert.Equal(t, 1, len(changes))
}

func TestChangedTouched(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	change := Change{At: now, Actor: "user"}
	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	update, _ := m.Apply(observation)

	// Changed updates only touch the entity itself
	assert.Equal(t, []EntityID{id}, update.Touched)
}

func TestChangedRevision(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	change := Change{
		At:       now,
		Actor:    "user",
		Revision: "rev-12345",
	}

	observation := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Change: change,
	}
	_, _ = m.Apply(observation)

	changes := m.Changes(id, time.Time{})
	assert.Equal(t, "rev-12345", changes[0].Revision)
}
