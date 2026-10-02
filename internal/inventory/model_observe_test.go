package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestObservedEntityPresent(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	observation := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
		UID:    "uid-12345",
		Attributes: map[string]Value{
			"status": Text("running"),
		},
	}

	update, err := m.Apply(observation)
	assert.NoError(t, err)
	assert.Equal(t, []EntityID{id}, update.Touched)

	// Entity should be present
	entity, ok := m.Entity(id)
	assert.True(t, ok)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, "uid-12345", entity.UID)
	assert.True(t, m.Exists(id))
}

func TestObservedUIDSet(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// First observation with UID
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
		UID:    "uid-1",
	}
	_, _ = m.Apply(observation1)

	entity, _ := m.Entity(id)
	assert.Equal(t, "uid-1", entity.UID)

	// Second observation updates UID
	observation2 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
		UID:    "uid-2",
	}
	_, _ = m.Apply(observation2)

	entity, _ = m.Entity(id)
	assert.Equal(t, "uid-2", entity.UID)

	// Observation without UID doesn't clear existing UID
	observation3 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now.Add(2 * time.Second),
		Entity: id,
	}
	_, _ = m.Apply(observation3)

	entity, _ = m.Entity(id)
	assert.Equal(t, "uid-2", entity.UID)
}

func TestObservedFirstSeenOnce(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
	}
	_, _ = m.Apply(observation1)

	entity1, _ := m.Entity(id)
	firstSeen1 := entity1.FirstSeen

	// Apply another Observed observation later
	observation2 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t2,
		Entity: id,
	}
	_, _ = m.Apply(observation2)

	entity2, _ := m.Entity(id)
	// FirstSeen should not change
	assert.Equal(t, firstSeen1, entity2.FirstSeen)
	assert.Equal(t, t1, entity2.FirstSeen)
}

func TestObservedAttributeSinceResetOnChange(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	t3 := t1.Add(2 * time.Minute)

	// First observation with attribute value "running"
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
		Attributes: map[string]Value{
			"status": Text("running"),
		},
	}
	_, _ = m.Apply(observation1)

	entity1, _ := m.Entity(id)
	attr1, _ := entity1.Attribute("status")
	assert.Equal(t, t1, attr1.Since)
	assert.Equal(t, t1, attr1.Updated)

	// Second observation with same value - Since stays, Updated advances
	observation2 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t2,
		Entity: id,
		Attributes: map[string]Value{
			"status": Text("running"),
		},
	}
	_, _ = m.Apply(observation2)

	entity2, _ := m.Entity(id)
	attr2, _ := entity2.Attribute("status")
	assert.Equal(t, t1, attr2.Since, "Since should not change when value is same")
	assert.Equal(t, t2, attr2.Updated, "Updated should advance")

	// Third observation with different value - Since resets
	observation3 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t3,
		Entity: id,
		Attributes: map[string]Value{
			"status": Text("stopped"),
		},
	}
	_, _ = m.Apply(observation3)

	entity3, _ := m.Entity(id)
	attr3, _ := entity3.Attribute("status")
	// Since should reset when value changes
	assert.Equal(t, t3, attr3.Since)
	assert.Equal(t, t3, attr3.Updated)
}

func TestObservedAttributeRemovalSameSource(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Observation with attribute
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Attributes: map[string]Value{
			"status":   Text("running"),
			"image":    Text("nginx:latest"),
			"restarts": Number(5),
		},
	}
	_, _ = m.Apply(observation1)

	entity1, _ := m.Entity(id)
	assert.Equal(t, 3, len(entity1.Attributes))

	// Observation from same source missing "image" - should be removed
	observation2 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now.Add(time.Second),
		Entity: id,
		Attributes: map[string]Value{
			"status":   Text("running"),
			"restarts": Number(6),
		},
	}
	_, _ = m.Apply(observation2)

	entity2, _ := m.Entity(id)
	assert.Equal(t, 2, len(entity2.Attributes))
	_, hasImage := entity2.Attribute("image")
	assert.False(t, hasImage)
	_, hasStatus := entity2.Attribute("status")
	assert.True(t, hasStatus)
	_, hasRestarts := entity2.Attribute("restarts")
	assert.True(t, hasRestarts)
}

func TestObservedAttributeKeptAcrossSources(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// First source adds "status"
	observation1 := Observation{
		Kind:   Observed,
		Source: "source1",
		At:     now,
		Entity: id,
		Attributes: map[string]Value{
			"status": Text("running"),
		},
	}
	_, _ = m.Apply(observation1)

	// Second source adds "image"
	observation2 := Observation{
		Kind:   Observed,
		Source: "source2",
		At:     now.Add(time.Second),
		Entity: id,
		Attributes: map[string]Value{
			"image": Text("nginx:latest"),
		},
	}
	_, _ = m.Apply(observation2)

	entity, _ := m.Entity(id)
	assert.Equal(t, 2, len(entity.Attributes))

	// First source removes "status", second source's "image" stays
	observation3 := Observation{
		Kind:       Observed,
		Source:     "source1",
		At:         now.Add(2 * time.Second),
		Entity:     id,
		Attributes: map[string]Value{},
	}
	_, _ = m.Apply(observation3)

	entity, _ = m.Entity(id)
	assert.Equal(t, 1, len(entity.Attributes))
	_, hasImage := entity.Attribute("image")
	assert.True(t, hasImage)
	_, hasStatus := entity.Attribute("status")
	assert.False(t, hasStatus)
}

func TestObservedEntitiesSorted(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Add entities in non-sorted order
	ids := []EntityID{
		CoreID("pod", "default", "z-pod"),
		CoreID("pod", "default", "a-pod"),
		CoreID("pod", "default", "m-pod"),
		CoreID("node", "default", "node-1"),
	}

	for _, id := range ids {
		observation := Observation{
			Kind:   Observed,
			Source: "k8s",
			At:     now,
			Entity: id,
		}
		_, _ = m.Apply(observation)
	}

	pods := m.Entities("pod")
	assert.Equal(t, 3, len(pods))
	assert.Equal(t, "a-pod", pods[0].Name)
	assert.Equal(t, "m-pod", pods[1].Name)
	assert.Equal(t, "z-pod", pods[2].Name)

	nodes := m.Entities("node")
	assert.Equal(t, 1, len(nodes))
}

func TestEntityIsDetachedCopy(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	observation := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     now,
		Entity: id,
		Attributes: map[string]Value{
			"status": Text("running"),
		},
	}
	_, _ = m.Apply(observation)

	// Get entity and mutate it
	entity1, _ := m.Entity(id)
	entity1.Attributes["status"] = Attribute{
		Value: Text("stopped"),
	}
	entity1.Attributes["new"] = Attribute{
		Value: Text("added"),
	}

	// Get entity again and verify original is unchanged
	entity2, _ := m.Entity(id)
	attr, _ := entity2.Attribute("status")
	assert.Equal(t, "running", attr.Value.AsText())
	_, hasNew := entity2.Attribute("new")
	assert.False(t, hasNew)
}

func TestObservedMultipleAttributeUpdates(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "default", "my-pod")
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Apply multiple updates to verify attribute behavior
	observation1 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1,
		Entity: id,
		Attributes: map[string]Value{
			"replicas": Number(3),
		},
	}
	_, _ = m.Apply(observation1)

	observation2 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1.Add(time.Second),
		Entity: id,
		Attributes: map[string]Value{
			"replicas": Number(3),
		},
	}
	_, _ = m.Apply(observation2)

	observation3 := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     t1.Add(2 * time.Second),
		Entity: id,
		Attributes: map[string]Value{
			"replicas": Number(5),
		},
	}
	_, _ = m.Apply(observation3)

	entity, _ := m.Entity(id)
	attr, _ := entity.Attribute("replicas")
	// Since resets when value changes: t1+2s (when value became 5)
	// Updated should be t1+2s
	assert.Equal(t, t1.Add(2*time.Second), attr.Since)
	assert.Equal(t, t1.Add(2*time.Second), attr.Updated)
}

func TestObservedInvalidEntity(t *testing.T) {
	m := NewModel(Options{})
	observation := Observation{
		Kind:   Observed,
		Source: "k8s",
		At:     time.Now(),
		Entity: EntityID{}, // Zero entity
	}
	_, err := m.Apply(observation)
	assert.Equal(t, ErrInvalidEntity, err)
}
