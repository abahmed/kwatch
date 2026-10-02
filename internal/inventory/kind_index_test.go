package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelKindNamespaceIndex(t *testing.T) {
	m := NewModel(Options{})
	observe := func(id EntityID, labels string) {
		t.Helper()
		_, err := m.Apply(Observation{
			Kind: Observed, At: testTime, Entity: id,
			Attributes: map[string]Value{"labels": Text(labels)},
		})
		require.NoError(t, err)
	}
	b := CoreID("pod", "shop", "b")
	a := CoreID("pod", "shop", "a")
	other := CoreID("pod", "billing", "a")
	observe(b, "app=web")
	observe(a, "app=api")
	observe(other, "app=api")
	observe(CoreID("deployment", "shop", "a"), "")

	assert.Equal(t, []EntityID{a, b}, m.EntitiesIn("pod", "shop"))
	assert.Equal(t, []EntityID{other, a}, m.CoreEntitiesNamed("pod", "a"))
	assert.Equal(t, map[EntityID]Value{
		a: Text("app=api"), b: Text("app=web"),
	}, m.AttributeIn("pod", "shop", "labels"))

	_, err := m.Apply(Observation{Kind: Gone, At: testTime, Entity: a})
	require.NoError(t, err)
	assert.Equal(t, []EntityID{b}, m.EntitiesIn("pod", "shop"))
	assert.Equal(t, []EntityID{other}, m.CoreEntitiesNamed("pod", "a"))
	assert.NotContains(t, m.AttributeIn("pod", "shop", "labels"), a)
	assert.Empty(t, m.EntitiesIn("pod", "nowhere"))
}
