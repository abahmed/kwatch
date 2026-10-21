package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGoneAttributeKeepsWhatTheDeletionHandedOver(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("rolebinding", "shop", "reader")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s", At: now,
		Entity:     id,
		Attributes: map[string]Value{"rbac.accounts": Text("shop/web")}})

	_, present := m.GoneAttribute(id, "rbac.accounts")
	assert.False(t, present, "a present entity has no tombstone attributes")

	_, _ = m.Apply(Observation{Kind: Gone, Source: "k8s",
		At: now.Add(time.Second), Entity: id,
		Attributes: map[string]Value{"rbac.accounts": Text("shop/web")}})

	value, ok := m.GoneAttribute(id, "rbac.accounts")
	assert.True(t, ok)
	assert.Equal(t, "shop/web", value.AsText())
	_, ok = m.GoneAttribute(id, "rbac.role")
	assert.False(t, ok)
	_, live := m.Attribute(id, "rbac.accounts")
	assert.False(t, live, "the entity itself keeps no attribute")
}

func TestGoneAttributeIsDroppedWhenTheEntityReturns(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("rolebinding", "shop", "reader")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s", At: now,
		Entity: id})
	_, _ = m.Apply(Observation{Kind: Gone, Source: "k8s", At: now,
		Entity:     id,
		Attributes: map[string]Value{"rbac.accounts": Text("shop/web")}})
	_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s",
		At: now.Add(time.Minute), Entity: id})

	_, ok := m.GoneAttribute(id, "rbac.accounts")
	assert.False(t, ok)
}

func TestGoneWithoutAttributesLeavesNone(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("rolebinding", "shop", "reader")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s", At: now,
		Entity: id})
	_, _ = m.Apply(Observation{Kind: Gone, Source: "k8s", At: now,
		Entity: id})

	_, ok := m.GoneAttribute(id, "rbac.accounts")
	assert.False(t, ok)
}
