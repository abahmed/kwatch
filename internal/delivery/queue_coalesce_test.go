package delivery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

func resolveJob(key string) deliverJob {
	job := incidentJob(key, "ns")
	job.incident.Status = notification.StatusResolved
	return job
}

func queuedKeys(queue chan deliverJob) []string {
	var keys []string
	for _, job := range drainQueuedJobs(queue) {
		keys = append(keys, job.key())
	}
	return keys
}

func TestOfferQueuedJobResolveDisplacesNewestOtherJob(t *testing.T) {
	queue := make(chan deliverJob, 2)
	require.True(t, offerQueuedJob(queue, incidentJob("a", "ns")).accepted)
	require.True(t, offerQueuedJob(queue, incidentJob("b", "ns")).accepted)

	result := offerQueuedJob(queue, resolveJob("c"))

	require.True(t, result.accepted, "a resolve is never dropped")
	require.NotNil(t, result.displaced)
	assert.Equal(t, "b", result.displaced.key())
	assert.Equal(t, []string{"a", "c"}, queuedKeys(queue))
}

func TestOfferQueuedJobNeverReplacesQueuedResolve(t *testing.T) {
	queue := make(chan deliverJob, 1)
	require.True(t, offerQueuedJob(queue, resolveJob("a")).accepted)
	reopened := incidentJob("a", "ns")
	reopened.incident.Revision = 5

	result := offerQueuedJob(queue, reopened)

	assert.False(t, result.accepted)
	assert.Equal(t, []string{"a"}, queuedKeys(queue))
}

func TestOfferQueuedJobRejectsResolveWhenOnlyResolvesQueued(t *testing.T) {
	queue := make(chan deliverJob, 1)
	require.True(t, offerQueuedJob(queue, resolveJob("a")).accepted)

	assert.False(t, offerQueuedJob(queue, resolveJob("b")).accepted)
}

func TestPendingQueueCoalescesByConversation(t *testing.T) {
	am := managerWithEntries([]providerEntry{{provider: &fakeProvider{}}})
	for revision := 1; revision <= 3; revision++ {
		m := *incidentJob("pod/web", "ns").incident
		m.Revision = revision
		am.NotifyIncident(m)
	}

	require.Len(t, am.pending, 1)
	assert.Equal(t, 3, am.pending[0].incident.Revision)
}

func TestPendingQueueOverflowDeadLettersOnce(t *testing.T) {
	am := managerWithEntries([]providerEntry{{provider: &fakeProvider{}}})
	for i := 0; i < channelCap; i++ {
		am.Notify("queued before start")
	}
	registry := metrics.DefaultRegistry()
	deadBefore := registry.DeliveryDeadLetters.Load()
	droppedBefore := registry.DeliveryPendingDropped.Load()

	am.NotifyIncident(*resolveJob("pod/web").incident)
	am.Notify("one too many")

	assert.Equal(t, int64(2), registry.DeliveryDeadLetters.Load()-deadBefore)
	assert.Equal(t, int64(2),
		registry.DeliveryPendingDropped.Load()-droppedBefore)
	require.Len(t, am.pending, channelCap)
	assert.True(t, am.pending[channelCap-1].isResolve(),
		"the resolve took the place of a plain message")
}

func TestStopBeforeStartDeadLettersPendingJobs(t *testing.T) {
	am := managerWithEntries([]providerEntry{{provider: &fakeProvider{}}})
	am.Notify("first")
	am.Notify("second")
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()

	require.NoError(t, am.Stop(context.Background()))

	assert.Equal(t, int64(2),
		metrics.DefaultRegistry().DeliveryDeadLetters.Load()-before)
}

func TestFanOutSettlesDisplacedOutboxRecord(t *testing.T) {
	entry := providerEntry{
		catalogName: "slack", provider: &fakeProvider{},
		ch: make(chan deliverJob, 1),
	}
	am := managerWithEntries([]providerEntry{entry})
	box := newOutbox(newMemOutbox(), am.nowTime)
	am.outbox.Store(box)
	am.mu.Lock()
	am.fanOut(incidentJob("a", "ns"))
	am.fanOut(resolveJob("b"))
	am.mu.Unlock()

	box.mu.Lock()
	defer box.mu.Unlock()
	assert.Len(t, box.live, 1, "only the queued resolve keeps a record")
}
