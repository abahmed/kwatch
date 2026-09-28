package knowledge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGoneRemovesEntityPresence(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe entity
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Attributes: map[string]Value{
			"status": Text("running"),
		},
	}
	_, _ = m.Apply(fact1)

	assert.True(t, m.Exists(id))
	_, ok := m.Entity(id)
	assert.True(t, ok)

	// Mark as gone
	fact2 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(fact2)

	assert.False(t, m.Exists(id))
	_, ok = m.Entity(id)
	assert.False(t, ok)
}

func TestGoneOutgoingEdgesRemoved(t *testing.T) {
	m := NewModel(Options{})
	podID := NewEntityID("pod", "default", "my-pod")
	pvcID := NewEntityID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe pod and pvc, add relation
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: podID,
	}
	_, _ = m.Apply(fact1)

	fact2 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: pvcID,
	}
	_, _ = m.Apply(fact2)

	fact3 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: Mounts,
		Targets:  []EntityID{pvcID},
	}
	_, _ = m.Apply(fact3)

	// Verify relation exists
	targets := m.Related(podID, Mounts, Outgoing)
	assert.Equal(t, []EntityID{pvcID}, targets)

	// Remove pod - its outgoing edges should be removed
	fact4 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: podID,
	}
	_, _ = m.Apply(fact4)

	// Pod's outgoing edges removed
	targets = m.Related(podID, Mounts, Outgoing)
	assert.Empty(t, targets)
}

func TestGoneIncomingEdgesKept(t *testing.T) {
	m := NewModel(Options{})
	secretID := NewEntityID("secret", "default", "my-secret")
	podID := NewEntityID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe entities
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: secretID,
	}
	_, _ = m.Apply(fact1)

	fact2 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: podID,
	}
	_, _ = m.Apply(fact2)

	// Pod references secret
	fact3 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: References,
		Targets:  []EntityID{secretID},
	}
	_, _ = m.Apply(fact3)

	// Remove secret
	fact4 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: secretID,
	}
	_, _ = m.Apply(fact4)

	// Pod's incoming reference to secret is kept
	incoming := m.Related(secretID, References, Incoming)
	assert.Equal(t, []EntityID{podID}, incoming)
}

func TestGoneCreatesDeletionChangeRecord(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe entity
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	_, _ = m.Apply(fact1)

	// Remove it
	fact2 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
		Change: Change{Actor: "system"},
	}
	_, _ = m.Apply(fact2)

	// Deleted entity keeps change history
	changes := m.Changes(id, time.Time{})
	assert.Equal(t, 1, len(changes))
	assert.True(t, changes[0].Deleted)
}

func TestGoneReobservedBecomesPresent(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	// Observe entity
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
	}
	_, _ = m.Apply(fact1)

	entity1, _ := m.Entity(id)
	firstSeen1 := entity1.FirstSeen

	// Mark as gone
	fact2 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     t1.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(fact2)

	assert.False(t, m.Exists(id))

	// Re-observe later
	fact3 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     t2,
		Entity: id,
	}
	_, _ = m.Apply(fact3)

	assert.True(t, m.Exists(id))
	entity3, _ := m.Entity(id)
	// FirstSeen should be updated to new observation time
	assert.Equal(t, t2, entity3.FirstSeen)
	assert.NotEqual(t, firstSeen1, entity3.FirstSeen)
}

func TestGoneUnknownEntity(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "unknown-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Mark unknown entity as gone - should return empty Update
	fact := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	update, _ := m.Apply(fact)
	assert.Empty(t, update.Touched)
}

func TestGoneTouched(t *testing.T) {
	m := NewModel(Options{})
	podID := NewEntityID("pod", "default", "my-pod")
	pvcID := NewEntityID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe pod and pvc, add relation
	fact1 := Fact{Kind: Observed, Source: "k8s", At: now, Entity: podID}
	_, _ = m.Apply(fact1)

	fact2 := Fact{Kind: Observed, Source: "k8s", At: now, Entity: pvcID}
	_, _ = m.Apply(fact2)

	fact3 := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: Mounts,
		Targets:  []EntityID{pvcID},
	}
	_, _ = m.Apply(fact3)

	// Remove pod
	fact4 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: podID,
	}
	update, _ := m.Apply(fact4)

	// Touched should include pod and pvc (target affected by edge removal)
	assert.Contains(t, update.Touched, podID)
	assert.Contains(t, update.Touched, pvcID)
}

func TestPruneRemovesOldChanges(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t1.Add(2 * time.Hour)
	t4 := t1.Add(3 * time.Hour)

	// Add changes at different times
	for _, ts := range []time.Time{t1, t2, t3, t4} {
		change := Change{At: ts, Actor: "user"}
		fact := Fact{
			Kind:   Changed,
			Source: "k8s",
			At:     ts,
			Entity: id,
			Change: change,
		}
		_, _ = m.Apply(fact)
	}

	changes := m.Changes(id, time.Time{})
	assert.Equal(t, 4, len(changes))

	// Prune changes before t3 - should remove t1 and t2
	removed := m.Prune(t3)
	assert.Equal(t, 2, removed)

	changes = m.Changes(id, time.Time{})
	assert.Equal(t, 2, len(changes))
	assert.Equal(t, t3, changes[0].At)
	assert.Equal(t, t4, changes[1].At)
}

func TestPruneDeletesTombstoneWithNoHistory(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe and then remove entity
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	_, _ = m.Apply(fact1)

	fact2 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(fact2)

	stats1 := m.Stats()
	assert.Equal(t, 0, stats1.Entities)
	assert.Equal(t, 1, stats1.Tombstones)

	// Prune changes before the Gone time
	m.Prune(now.Add(time.Hour))

	stats2 := m.Stats()
	// Tombstone should be deleted
	assert.Equal(t, 0, stats2.Entities)
	assert.Equal(t, 0, stats2.Tombstones)
}

func TestPruneKeepsTombstoneWithChanges(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)

	// Observe entity
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
	}
	_, _ = m.Apply(fact1)

	// Add change
	fact2 := Fact{
		Kind:   Changed,
		Source: "k8s",
		At:     t2,
		Entity: id,
		Change: Change{At: t2, Actor: "user"},
	}
	_, _ = m.Apply(fact2)

	// Mark as gone
	fact3 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     t3,
		Entity: id,
	}
	_, _ = m.Apply(fact3)

	// Prune before t2 - the change at t2 should be removed
	m.Prune(t2.Add(time.Microsecond))

	// But tombstone should still exist because Gone change is recorded
	stats := m.Stats()
	assert.Equal(t, 1, stats.Tombstones)

	// Prune before Gone time
	m.Prune(t3.Add(time.Hour))

	stats = m.Stats()
	// Now tombstone should be deleted
	assert.Equal(t, 0, stats.Tombstones)
}

func TestPruneKeepsRecordsWithRelations(t *testing.T) {
	m := NewModel(Options{})
	fromID := NewEntityID("pod", "default", "my-pod")
	toID := NewEntityID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add relation without observing fromID
	fact := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{toID},
	}
	_, _ = m.Apply(fact)

	stats1 := m.Stats()
	// fromID exists as a record (with relation) but not present
	assert.Equal(t, 0, stats1.Entities)
	assert.Equal(t, 1, stats1.Relations)

	// Prune should NOT delete this record because it has relations
	m.Prune(now.Add(time.Hour))

	stats2 := m.Stats()
	// Record should still exist
	assert.Equal(t, 1, stats2.Relations)
}

func TestPruneBoundedByGoneTime(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Second)
	t3 := t1.Add(time.Hour)

	// Observe
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
	}
	_, _ = m.Apply(fact1)

	// Mark as gone
	fact2 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     t2,
		Entity: id,
	}
	_, _ = m.Apply(fact2)

	stats1 := m.Stats()
	assert.Equal(t, 1, stats1.Tombstones)

	// Prune at time before Gone - tombstone should remain
	m.Prune(t2.Add(-time.Second))

	stats2 := m.Stats()
	assert.Equal(t, 1, stats2.Tombstones)

	// Prune after Gone - tombstone should be deleted
	m.Prune(t3)

	stats3 := m.Stats()
	assert.Equal(t, 0, stats3.Tombstones)
}

func TestPruneReturnValue(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add 3 changes
	for i := 0; i < 3; i++ {
		change := Change{At: t1.Add(time.Duration(i) * time.Second)}
		fact := Fact{
			Kind:   Changed,
			Source: "k8s",
			At:     t1.Add(time.Duration(i) * time.Second),
			Entity: id,
			Change: change,
		}
		_, _ = m.Apply(fact)
	}

	// Prune should remove 2 changes
	removed := m.Prune(t1.Add(2 * time.Second))
	assert.Equal(t, 2, removed)
}

func TestGoneInvalidEntity(t *testing.T) {
	m := NewModel(Options{})
	fact := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     time.Now(),
		Entity: EntityID{}, // Zero entity
	}
	_, err := m.Apply(fact)
	assert.Equal(t, ErrInvalidEntity, err)
}

func TestStatsAfterGone(t *testing.T) {
	m := NewModel(Options{})
	id := NewEntityID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe
	fact1 := Fact{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	_, _ = m.Apply(fact1)

	stats1 := m.Stats()
	assert.Equal(t, 1, stats1.Entities)
	assert.Equal(t, 0, stats1.Tombstones)

	// Mark as gone
	fact2 := Fact{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(fact2)

	stats2 := m.Stats()
	assert.Equal(t, 0, stats2.Entities)
	assert.Equal(t, 1, stats2.Tombstones)
}
