package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var enrichmentAt = time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

func enrichmentModel() *Model {
	return NewModel(Options{EnrichmentSources: []string{"kubelet"}})
}

func statsObservation(id EntityID, at time.Time) Observation {
	return Observation{
		Kind: Observed, Source: "kubelet", At: at, Entity: id,
		Attributes: map[string]Value{"cpu": Number(250)},
	}
}

func authoritativeObservation(id EntityID, at time.Time) Observation {
	return Observation{
		Kind: Observed, Source: "kubernetes", At: at, Entity: id,
		Attributes: map[string]Value{"phase": Text("Running")},
	}
}

func TestModelEnrichmentUpdatesPresentEntity(t *testing.T) {
	m := enrichmentModel()
	id := CoreID("pod", "ns", "p")
	_, err := m.Apply(authoritativeObservation(id, enrichmentAt))
	require.NoError(t, err)

	update, err := m.Apply(statsObservation(id, enrichmentAt.Add(time.Minute)))

	require.NoError(t, err)
	assert.Equal(t, []EntityID{id}, update.Touched)
	entity, ok := m.Entity(id)
	require.True(t, ok)
	assert.Contains(t, entity.Attributes, "cpu")
	assert.Contains(t, entity.Attributes, "phase")
}

func TestModelEnrichmentDoesNotCreateUnknownEntity(t *testing.T) {
	m := enrichmentModel()
	id := CoreID("pod", "ns", "never-seen")

	update, err := m.Apply(statsObservation(id, enrichmentAt))

	require.NoError(t, err)
	assert.Empty(t, update.Touched)
	assert.False(t, m.Exists(id))
	assert.Empty(t, m.Entities("pod"))
}

func TestModelEnrichmentDoesNotResurrectGoneEntity(t *testing.T) {
	m := enrichmentModel()
	id := CoreID("pod", "ns", "p")
	_, err := m.Apply(authoritativeObservation(id, enrichmentAt))
	require.NoError(t, err)
	_, err = m.Apply(Observation{Kind: Gone, Source: "kubernetes",
		At: enrichmentAt.Add(time.Minute), Entity: id})
	require.NoError(t, err)

	update, err := m.Apply(statsObservation(id, enrichmentAt.Add(2*time.Minute)))

	require.NoError(t, err)
	assert.Empty(t, update.Touched)
	assert.False(t, m.Exists(id), "stats must not resurrect a gone pod")
	assert.Empty(t, m.Entities("pod"))
}

// An authoritative observation after deletion (a pod recreated under the
// same name) brings the entity back, and enrichment applies again.
func TestModelEnrichmentResumesAfterAuthoritativeRecreate(t *testing.T) {
	m := enrichmentModel()
	id := CoreID("pod", "ns", "p")
	_, _ = m.Apply(authoritativeObservation(id, enrichmentAt))
	_, _ = m.Apply(Observation{Kind: Gone, Source: "kubernetes",
		At: enrichmentAt.Add(time.Minute), Entity: id})
	_, _ = m.Apply(statsObservation(id, enrichmentAt.Add(2*time.Minute)))

	_, err := m.Apply(authoritativeObservation(
		id, enrichmentAt.Add(3*time.Minute)))
	require.NoError(t, err)
	_, err = m.Apply(statsObservation(id, enrichmentAt.Add(4*time.Minute)))
	require.NoError(t, err)

	entity, ok := m.Entity(id)
	require.True(t, ok)
	assert.Contains(t, entity.Attributes, "cpu")
}

func TestModelWithoutEnrichmentSourcesKeepsObservedSemantics(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "ns", "p")

	_, err := m.Apply(statsObservation(id, enrichmentAt))

	require.NoError(t, err)
	assert.True(t, m.Exists(id))
}
