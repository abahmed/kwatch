package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// observeBool delivers one boolean attribute value at at.
func observeBool(m *Model, id EntityID, at time.Time, v bool) {
	_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s", At: at,
		Entity: id, Attributes: map[string]Value{"ready": Bool(v)}})
}

func TestBoolAttributeRemembersItsFlips(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "shop", "api-0")
	start := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)

	observeBool(m, id, start, true)
	observeBool(m, id, start.Add(time.Minute), true)
	observeBool(m, id, start.Add(2*time.Minute), false)
	observeBool(m, id, start.Add(3*time.Minute), true)

	entity, _ := m.Entity(id)
	attribute, _ := entity.Attribute("ready")
	assert.Equal(t, []time.Time{start.Add(2 * time.Minute),
		start.Add(3 * time.Minute)}, attribute.Flips)
}

func TestFlipsAreBounded(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "shop", "api-0")
	start := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 5*MaxAttributeFlips; i++ {
		observeBool(m, id, start.Add(time.Duration(i)*time.Second), i%2 == 0)
	}

	entity, _ := m.Entity(id)
	attribute, _ := entity.Attribute("ready")
	assert.Len(t, attribute.Flips, MaxAttributeFlips)
	lastAt := start.Add(time.Duration(5*MaxAttributeFlips-1) * time.Second)
	assert.Equal(t, lastAt, attribute.Flips[MaxAttributeFlips-1])
}

func TestTextAttributeKeepsNoFlips(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "shop", "api-0")
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	for i, phase := range []string{"Pending", "Running"} {
		_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s",
			At: now.Add(time.Duration(i) * time.Second), Entity: id,
			Attributes: map[string]Value{"phase": Text(phase)}})
	}

	entity, _ := m.Entity(id)
	attribute, _ := entity.Attribute("phase")
	assert.Empty(t, attribute.Flips)
}
