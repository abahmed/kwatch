package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A relation declared for an entity after it is gone (a join on another
// watch that raced the delete) leaves no edge behind, and the tombstone
// is pruned like any other.
func TestRelatedAfterGoneLeavesNoEdgeAndIsPruned(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "shop", "web-1")
	endpoint := CoreID("endpoint", "shop", "pay")
	at := testTime
	for _, o := range []Observation{
		{Kind: Observed, Source: "kubernetes", At: at, Entity: pod},
		{Kind: Observed, Source: "kubernetes", At: at, Entity: endpoint},
		{Kind: Gone, Source: "kubernetes", At: at.Add(time.Second),
			Entity: pod},
		{Kind: Related, Source: "config", At: at.Add(2 * time.Second),
			Entity: pod, Relation: Calls, Targets: []EntityID{endpoint}},
	} {
		_, err := m.Apply(o)
		assert.NoError(t, err)
	}
	assert.Empty(t, m.Related(endpoint, Calls, Incoming))
	assert.Empty(t, m.Related(pod, Calls, Outgoing))
	assert.Equal(t, 0, m.Stats().Relations)

	m.Prune(at.Add(time.Hour))
	assert.Equal(t, 0, m.Stats().Tombstones)
}

// A re-created entity is observed again before its relations arrive, so
// they are kept.
func TestRelatedAfterReturnIsKept(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "shop", "web-0")
	endpoint := CoreID("endpoint", "shop", "pay")
	at := testTime
	for _, o := range []Observation{
		{Kind: Observed, Source: "kubernetes", At: at, Entity: pod},
		{Kind: Gone, Source: "kubernetes", At: at.Add(time.Second),
			Entity: pod},
		{Kind: Observed, Source: "kubernetes", At: at.Add(2 * time.Second),
			Entity: pod},
		{Kind: Related, Source: "config", At: at.Add(3 * time.Second),
			Entity: pod, Relation: Calls, Targets: []EntityID{endpoint}},
	} {
		_, err := m.Apply(o)
		assert.NoError(t, err)
	}
	assert.Equal(t, []EntityID{pod}, m.Related(endpoint, Calls, Incoming))
}

// A relation declared before the entity was ever observed is kept, as
// before: only a gone entity is refused.
func TestRelatedBeforeFirstObservationIsKept(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "shop", "web-2")
	endpoint := CoreID("endpoint", "shop", "pay")
	_, err := m.Apply(Observation{Kind: Related, Source: "config",
		At: testTime, Entity: pod, Relation: Calls,
		Targets: []EntityID{endpoint}})
	assert.NoError(t, err)
	assert.Equal(t, []EntityID{pod}, m.Related(endpoint, Calls, Incoming))
}
