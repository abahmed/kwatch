package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func webhookModel(t *testing.T) (*inventory.Model, inventory.EntityID) {
	t.Helper()
	model := inventory.NewModel(inventory.Options{
		EnrichmentSources: EnrichmentSources(),
	})
	id := inventory.CoreID(KindValidatingHook, "", "policy")
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: ObservationSource, At: planeStart,
		Entity: id, Attributes: map[string]inventory.Value{
			AttrWebhookNames: inventory.Text("opa.example,other.example"),
		},
	})
	require.NoError(t, err)
	return model, id
}

func TestWebhookObservationsDescribeTheConfigurationByItsHooks(t *testing.T) {
	model, id := webhookModel(t)
	p := NewProber(ProbeConfig{Model: model})
	readPlane(t, p, planeBody{process: 1}, planeStart)

	_, obs := readPlane(t, p,
		planeBody{process: 1, hookCalls: 50, hookClosed: 20},
		planeStart.Add(30*time.Second))

	require.Len(t, obs, 1)
	assert.Equal(t, id, obs[0].Entity)
	assert.Equal(t, webhookMetricsSource, obs[0].Source)
	attrs := obs[0].Attributes
	assert.Equal(t, "opa.example", attrs[AttrWebhookSlowest].AsText())
	closed, _ := attrs[AttrWebhookClosedShare].AsNumber()
	assert.InDelta(t, 40.0, closed, 0.001)
	open, _ := attrs[AttrWebhookOpenShare].AsNumber()
	assert.Zero(t, open)
	p99, _ := attrs[AttrWebhookP99].AsNumber()
	assert.Greater(t, p99, 1000.0)
}

func TestWebhookObservationsClearWhenTheHookIsNotCalled(t *testing.T) {
	model, id := webhookModel(t)
	p := NewProber(ProbeConfig{Model: model})
	readPlane(t, p, planeBody{process: 1, hookCalls: 50}, planeStart)

	_, obs := readPlane(t, p, planeBody{process: 1, hookCalls: 52},
		planeStart.Add(30*time.Second))

	require.Len(t, obs, 1)
	assert.Equal(t, id, obs[0].Entity)
	assert.Empty(t, obs[0].Attributes, "too few calls to say anything")
}

func TestWebhookObservationsNeedTheModel(t *testing.T) {
	p := NewProber(ProbeConfig{})
	readPlane(t, p, planeBody{process: 1}, planeStart)

	_, obs := readPlane(t, p, planeBody{process: 1, hookCalls: 50},
		planeStart.Add(30*time.Second))

	assert.Empty(t, obs)
}

func TestWebhookMetricsNeverCreateAConfiguration(t *testing.T) {
	model := inventory.NewModel(inventory.Options{
		EnrichmentSources: EnrichmentSources(),
	})
	id := inventory.CoreID(KindValidatingHook, "", "gone")

	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: webhookMetricsSource,
		At: planeStart, Entity: id,
		Attributes: map[string]inventory.Value{
			AttrWebhookP99: inventory.Number(1),
		},
	})

	require.NoError(t, err)
	assert.False(t, model.Exists(id))
}
