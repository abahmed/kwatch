package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGoneRemovesEntityPresence(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe entity
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Attributes: map[string]Value{
			"status": Text("running"),
		},
	}
	_, _ = m.Apply(observation1)

	assert.True(t, m.Exists(id))
	_, ok := m.Entity(id)
	assert.True(t, ok)

	// Mark as gone
	observation2 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(observation2)

	assert.False(t, m.Exists(id))
	_, ok = m.Entity(id)
	assert.False(t, ok)
}

func TestGoneOutgoingEdgesRemoved(t *testing.T) {
	m := NewModel(Options{})
	podID := CoreID("pod", "default", "my-pod")
	pvcID := CoreID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe pod and pvc, add relation
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: podID,
	}
	_, _ = m.Apply(observation1)

	observation2 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: pvcID,
	}
	_, _ = m.Apply(observation2)

	observation3 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: Mounts,
		Targets:  []EntityID{pvcID},
	}
	_, _ = m.Apply(observation3)

	// Verify relation exists
	targets := m.Related(podID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{pvcID}, targets)

	// Remove pod - its outgoing edges should be removed
	observation4 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: podID,
	}
	_, _ = m.Apply(observation4)

	// Pod's outgoing edges removed
	targets = m.Related(podID, Mounts, Outgoing)
	assert.Empty(t, targets)
}

func TestGoneIncomingEdgesKept(t *testing.T) {
	m := NewModel(Options{})
	secretID := CoreID("secret", "default", "my-secret")
	podID := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe entities
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: secretID,
	}
	_, _ = m.Apply(observation1)

	observation2 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: podID,
	}
	_, _ = m.Apply(observation2)

	// Pod references secret
	observation3 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: References,
		Targets:  []EntityID{secretID},
	}
	_, _ = m.Apply(observation3)

	// Remove secret
	observation4 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: secretID,
	}
	_, _ = m.Apply(observation4)

	// Pod's incoming reference to secret is kept
	incoming := m.Related(secretID, References, Incoming)
	assert.Equal(t, []EntityID{podID}, incoming)
}

func TestGoneCreatesDeletionChangeRecord(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe entity
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	_, _ = m.Apply(observation1)

	// Remove it
	observation2 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
		Change: Change{Actor: "system"},
	}
	_, _ = m.Apply(observation2)

	// Deleted entity keeps change history
	changes := m.Changes(id, time.Time{})
	assert.Equal(t, 1, len(changes))
	assert.True(t, changes[0].Deleted)
}

func TestGoneReobservedBecomesPresent(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	// Observe entity
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
	}
	_, _ = m.Apply(observation1)

	entity1, _ := m.Entity(id)
	firstSeen1 := entity1.FirstSeen

	// Mark as gone
	observation2 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     t1.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(observation2)

	assert.False(t, m.Exists(id))

	// Re-observe later
	observation3 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t2,
		Entity: id,
	}
	_, _ = m.Apply(observation3)

	assert.True(t, m.Exists(id))
	entity3, _ := m.Entity(id)
	// FirstSeen should be updated to new observation time
	assert.Equal(t, t2, entity3.FirstSeen)
	assert.NotEqual(t, firstSeen1, entity3.FirstSeen)
}

func TestGoneUnknownEntity(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "unknown-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Mark unknown entity as gone - should return empty Update
	observation := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	update, _ := m.Apply(observation)
	assert.Empty(t, update.Touched)
}

func TestGoneTouched(t *testing.T) {
	m := NewModel(Options{})
	podID := CoreID("pod", "default", "my-pod")
	pvcID := CoreID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe pod and pvc, add relation
	observation1 := Observation{
		Kind: Observed, Source: "k8s", At: now, Entity: podID,
	}
	_, _ = m.Apply(observation1)

	observation2 := Observation{
		Kind: Observed, Source: "k8s", At: now, Entity: pvcID,
	}
	_, _ = m.Apply(observation2)

	observation3 := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: Mounts,
		Targets:  []EntityID{pvcID},
	}
	_, _ = m.Apply(observation3)

	// Remove pod
	observation4 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: podID,
	}
	update, _ := m.Apply(observation4)

	// Touched should include pod and pvc (target affected by edge removal)
	assert.Contains(t, update.Touched, podID)
	assert.Contains(t, update.Touched, pvcID)
}

func TestGoneInvalidEntity(t *testing.T) {
	m := NewModel(Options{})
	observation := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     time.Now(),
		Entity: EntityID{}, // Zero entity
	}
	_, err := m.Apply(observation)
	assert.Equal(t, ErrInvalidEntity, err)
}

func TestStatsAfterGone(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	_, _ = m.Apply(observation1)

	stats1 := m.Stats()
	assert.Equal(t, 1, stats1.Entities)
	assert.Equal(t, 0, stats1.Tombstones)

	// Mark as gone
	observation2 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(observation2)

	stats2 := m.Stats()
	assert.Equal(t, 0, stats2.Entities)
	assert.Equal(t, 1, stats2.Tombstones)
}
