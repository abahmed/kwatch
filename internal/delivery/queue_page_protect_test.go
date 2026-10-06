package delivery

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/metrics"
)

func pageAnnounce(key string) deliverJob {
	job := incidentJob(key, "ns")
	job.incident.Route.Severity = "critical"
	job.incident.Revision = 1
	return job
}

// In a full queue a resolve takes the place of a routine job even when
// a page announcement of another incident was queued after it.
func TestResolveDisplacesRoutineJobBeforePageAnnounce(t *testing.T) {
	queue := make(chan deliverJob, 2)
	require.True(t, offerQueuedJob(queue, incidentJob("routine", "ns")).accepted)
	require.True(t, offerQueuedJob(queue, pageAnnounce("page")).accepted)

	result := offerQueuedJob(queue, resolveJob("other"))

	require.True(t, result.accepted)
	require.NotNil(t, result.displaced)
	assert.Equal(t, "routine", result.displaced.key())
	assert.ElementsMatch(t, []string{"other", "page"}, queuedKeys(queue))
}

// With only page announcements and resolves queued, a new resolve is not
// admitted: it must not cancel the opening of another alert.
func TestResolveNeverDisplacesPageAnnounce(t *testing.T) {
	queue := make(chan deliverJob, 2)
	require.True(t, offerQueuedJob(queue, pageAnnounce("p1")).accepted)
	require.True(t, offerQueuedJob(queue, resolveJob("r1")).accepted)

	result := offerQueuedJob(queue, resolveJob("r2"))

	assert.False(t, result.accepted)
	assert.Nil(t, result.displaced)
	assert.ElementsMatch(t, []string{"p1", "r1"}, queuedKeys(queue))
}

// A resolve that is given up on is counted and logged as lost; any
// other job is not.
func TestDeadLetteredResolveIsCountedAsLost(t *testing.T) {
	m := newTestManager()
	lost := &metrics.DefaultRegistry().DeliveryResolvesLost
	before := lost.Load()

	m.recordDeadLetter("Pager", incidentJob("a", "ns"), errors.New("x"))
	assert.Equal(t, before, lost.Load())

	resolve := resolveJob("a")
	resolve.incident.DedupKey = "alert-a"
	m.recordDeadLetter("Pager", resolve, errJobExpired)
	assert.Equal(t, before+1, lost.Load())
}
