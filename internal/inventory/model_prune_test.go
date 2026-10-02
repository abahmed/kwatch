package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPruneRemovesOldChanges(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t1.Add(2 * time.Hour)
	t4 := t1.Add(3 * time.Hour)

	// Add changes at different times
	for _, ts := range []time.Time{t1, t2, t3, t4} {
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
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observe and then remove entity
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
	}
	_, _ = m.Apply(observation1)

	observation2 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
	}
	_, _ = m.Apply(observation2)

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
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)

	// Observe entity
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
	}
	_, _ = m.Apply(observation1)

	// Add change
	observation2 := Observation{
		Kind:   Changed,
		Source: "k8s",
		At:     t2,
		Entity: id,
		Change: Change{At: t2, Actor: "user"},
	}
	_, _ = m.Apply(observation2)

	// Mark as gone
	observation3 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     t3,
		Entity: id,
	}
	_, _ = m.Apply(observation3)

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
	fromID := CoreID("pod", "default", "my-pod")
	toID := CoreID("pvc", "default", "my-pvc")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add relation without observing fromID
	observation := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   fromID,
		Relation: Mounts,
		Targets:  []EntityID{toID},
	}
	_, _ = m.Apply(observation)

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
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Second)
	t3 := t1.Add(time.Hour)

	// Observe
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
	}
	_, _ = m.Apply(observation1)

	// Mark as gone
	observation2 := Observation{
		Kind:   Gone,
		Source: "k8s",
		At:     t2,
		Entity: id,
	}
	_, _ = m.Apply(observation2)

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
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add 3 changes
	for i := 0; i < 3; i++ {
		change := Change{At: t1.Add(time.Duration(i) * time.Second)}
		observation := Observation{
			Kind:   Changed,
			Source: "k8s",
			At:     t1.Add(time.Duration(i) * time.Second),
			Entity: id,
			Change: change,
		}
		_, _ = m.Apply(observation)
	}

	// Prune should remove 2 changes
	removed := m.Prune(t1.Add(2 * time.Second))
	assert.Equal(t, 2, removed)
}
