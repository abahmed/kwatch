package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func attributeModel(t *testing.T) (*Model, EntityID) {
	t.Helper()
	m := NewModel(Options{})
	id := CoreID("pod", "shop", "api-1")
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	_, err := m.Apply(Observation{
		Kind: Observed, Source: "k", At: at, Entity: id,
		Attributes: map[string]Value{
			"phase": Text("Running"), "restarts": Number(3),
		},
	})
	assert.NoError(t, err)
	return m, id
}

func TestModelAttributeReturnsOneAttribute(t *testing.T) {
	m, id := attributeModel(t)
	got, ok := m.Attribute(id, "phase")
	assert.True(t, ok)
	assert.Equal(t, "Running", got.Value.AsText())
	entity, _ := m.Entity(id)
	assert.Equal(t, entity.Attributes["phase"], got)
}

func TestModelAttributeMissingIsNotFound(t *testing.T) {
	m, id := attributeModel(t)
	_, ok := m.Attribute(id, "nope")
	assert.False(t, ok, "unknown attribute name")
	_, ok = m.Attribute(CoreID("pod", "shop", "other"), "phase")
	assert.False(t, ok, "unknown entity")
}

func TestModelAttributeOfGoneEntityIsNotFound(t *testing.T) {
	m, id := attributeModel(t)
	_, err := m.Apply(Observation{
		Kind: Gone, Source: "k", At: time.Date(2026, 10, 1, 9, 1, 0, 0,
			time.UTC),
		Entity: id,
	})
	assert.NoError(t, err)
	_, ok := m.Attribute(id, "phase")
	assert.False(t, ok, "a deleted entity has no attributes")
}

func TestModelAttributeDoesNotAllocate(t *testing.T) {
	m, id := attributeModel(t)
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = m.Attribute(id, "restarts")
	})
	assert.Zero(t, allocs)
}
