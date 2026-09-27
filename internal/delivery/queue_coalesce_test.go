package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/model"
)

func TestQueueCoalescesIncidentUpdates(t *testing.T) {
	queue := make(chan deliverJob, 2)
	queue <- queueIncident("api", model.ActionCreate, 1)
	queue <- queueIncident("api", model.ActionUpdate, 2)
	accepted, dropped := offerQueuedJob(
		queue, queueIncident("api", model.ActionUpdate, 3),
	)
	assert.True(t, accepted)
	assert.Nil(t, dropped)
	assert.Equal(t, model.ActionCreate, (<-queue).action)
	latest := <-queue
	assert.Equal(t, model.ActionUpdate, latest.action)
	assert.Equal(t, uint64(3), latest.inc.Revision)
}

func TestQueueRefreshesUndeliveredCreate(t *testing.T) {
	queue := make(chan deliverJob, 1)
	queue <- queueIncident("api", model.ActionCreate, 1)
	accepted, dropped := offerQueuedJob(
		queue, queueIncident("api", model.ActionUpdate, 2),
	)
	assert.True(t, accepted)
	assert.Nil(t, dropped)
	latest := <-queue
	assert.Equal(t, model.ActionCreate, latest.action)
	assert.Equal(t, uint64(2), latest.inc.Revision)
}

func TestQueueKeepsCreateBeforeRecovery(t *testing.T) {
	queue := make(chan deliverJob, 2)
	queue <- queueIncident("api", model.ActionCreate, 1)
	queue <- queueIncident("api", model.ActionUpdate, 2)
	accepted, dropped := offerQueuedJob(
		queue, queueIncident("api", model.ActionResolved, 3),
	)
	assert.True(t, accepted)
	assert.Nil(t, dropped)
	assert.Equal(t, model.ActionCreate, (<-queue).action)
	assert.Equal(t, model.ActionResolved, (<-queue).action)
}

func TestQueuePrioritizesRecoveryOverUnrelatedUpdate(t *testing.T) {
	queue := make(chan deliverJob, 2)
	queue <- queueIncident("api", model.ActionCreate, 1)
	queue <- queueIncident("other", model.ActionUpdate, 2)
	accepted, dropped := offerQueuedJob(
		queue, queueIncident("api", model.ActionResolved, 3),
	)
	require.True(t, accepted)
	require.NotNil(t, dropped)
	assert.Equal(t, model.IncidentKey("other"), dropped.inc.Key)
	assert.Equal(t, model.ActionCreate, (<-queue).action)
	assert.Equal(t, model.ActionResolved, (<-queue).action)
}

func TestQueuePreservesCreatesWhenNoUpdateCanBeReplaced(t *testing.T) {
	queue := make(chan deliverJob, 2)
	queue <- queueIncident("first", model.ActionCreate, 1)
	queue <- queueIncident("second", model.ActionCreate, 1)
	accepted, dropped := offerQueuedJob(
		queue, queueIncident("third", model.ActionResolved, 2),
	)
	assert.False(t, accepted)
	assert.Nil(t, dropped)
	assert.Equal(t, model.IncidentKey("first"), (<-queue).inc.Key)
	assert.Equal(t, model.IncidentKey("second"), (<-queue).inc.Key)
}

func queueIncident(
	key string,
	action model.IncidentAction,
	revision uint64,
) deliverJob {
	return incidentJob(&model.Incident{
		Subject:  model.Subject{Key: model.IncidentKey(key)},
		Delivery: model.Delivery{Revision: revision},
	}, action, nil)
}
