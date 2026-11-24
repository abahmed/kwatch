package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// An endpoint nobody reads any more leaves nothing behind in the join:
// churning pods and ConfigMaps must not grow its memory without end.
func TestConfigJoinForgetsEndpointsNobodyReads(t *testing.T) {
	h := newJoinHarness(t)
	h.addMap(sharedConfigMap())
	reader := readerPod("api-1", keyRef("api-config", "DB_HOST"))
	h.apply(h.pods.Added(reader, true, joinAt))
	assert.Equal(t, []string{"db.example.com:5432"}, h.calls("api-1"))

	h.apply(h.pods.Deleted(reader, joinAt))

	join := h.pods.join
	assert.Empty(t, join.said)
	assert.Empty(t, join.told)
}

// An endpoint only the join described leaves the model once no pod
// reads it through a ConfigMap: nothing else would ever remove it.
func TestConfigJoinRetiresEndpointsOnlyItDescribed(t *testing.T) {
	h := newJoinHarness(t)
	h.pods.join.WithModel(h.model)
	h.addMap(sharedConfigMap())
	reader := readerPod("api-1", keyRef("api-config", "DB_HOST"))
	h.apply(h.pods.Added(reader, true, joinAt))
	endpoint := endpointOf("db.example.com:5432")
	require.True(t, h.model.Exists(endpoint))

	h.apply(h.pods.Deleted(reader, joinAt))

	assert.False(t, h.model.Exists(endpoint))
}

// An endpoint an active prober describes too keeps its entity and the
// prober's attributes: the prober retires it when nobody calls it.
func TestConfigJoinLeavesAProbedEndpointToTheProber(t *testing.T) {
	h := newJoinHarness(t)
	h.pods.join.WithModel(h.model)
	h.addMap(sharedConfigMap())
	reader := readerPod("api-1", keyRef("api-config", "DB_HOST"))
	h.apply(h.pods.Added(reader, true, joinAt))
	endpoint := endpointOf("db.example.com:5432")
	h.apply([]inventory.Observation{{Kind: inventory.Observed,
		Source: dependencyProbeSource, At: joinAt, Entity: endpoint,
		Attributes: map[string]inventory.Value{
			AttrHealthy: inventory.Bool(true)}}})

	h.apply(h.pods.Deleted(reader, joinAt))

	got, ok := h.model.Entity(endpoint)
	require.True(t, ok)
	assert.Contains(t, got.Attributes, AttrHealthy)
	assert.NotContains(t, got.Attributes, AttrEndpointSources)
}

// Whether another source still describes an endpoint was read from the
// model when the join let go of it. When that source lets go too, the
// next pass of the join removes the endpoint: nothing else would.
func TestConfigJoinRechecksAnEndpointItLeftToAnother(t *testing.T) {
	h := newJoinHarness(t)
	h.pods.join.WithModel(h.model)
	h.addMap(sharedConfigMap())
	reader := readerPod("api-1", keyRef("api-config", "DB_HOST"))
	h.apply(h.pods.Added(reader, true, joinAt))
	endpoint := endpointOf("db.example.com:5432")
	h.apply([]inventory.Observation{{Kind: inventory.Observed,
		Source: dependencyProbeSource, At: joinAt, Entity: endpoint,
		Attributes: map[string]inventory.Value{
			AttrHealthy: inventory.Bool(true)}}})
	h.apply(h.pods.Deleted(reader, joinAt))
	require.True(t, h.model.Exists(endpoint))

	h.apply([]inventory.Observation{{Kind: inventory.Observed,
		Source: dependencyProbeSource, At: joinAt, Entity: endpoint}})
	h.apply(h.pods.Added(readerPod("web-1"), true, joinAt))

	assert.False(t, h.model.Exists(endpoint))
	assert.Empty(t, h.pods.join.withdrawn)
}

// An endpoint a ConfigMap names again is the join's again: no recheck
// removes it.
func TestConfigJoinKeepsAWithdrawnEndpointNamedAgain(t *testing.T) {
	h := newJoinHarness(t)
	h.pods.join.WithModel(h.model)
	h.addMap(sharedConfigMap())
	reader := readerPod("api-1", keyRef("api-config", "DB_HOST"))
	h.apply(h.pods.Added(reader, true, joinAt))
	endpoint := endpointOf("db.example.com:5432")
	h.apply([]inventory.Observation{{Kind: inventory.Observed,
		Source: dependencyProbeSource, At: joinAt, Entity: endpoint,
		Attributes: map[string]inventory.Value{
			AttrHealthy: inventory.Bool(true)}}})
	h.apply(h.pods.Deleted(reader, joinAt))
	h.apply(h.pods.Added(readerPod("api-2",
		keyRef("api-config", "DB_HOST")), true, joinAt))
	h.apply([]inventory.Observation{{Kind: inventory.Observed,
		Source: dependencyProbeSource, At: joinAt, Entity: endpoint}})
	h.apply(h.pods.Added(readerPod("web-1"), true, joinAt))

	assert.True(t, h.model.Exists(endpoint))
	assert.Empty(t, h.pods.join.withdrawn)
}
