package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/metrics"
)

// A page announcement in a full queue takes the place of a routine job,
// as a resolve does, instead of being refused.
func TestPageAnnounceDisplacesRoutineJobInFullQueue(t *testing.T) {
	queue := make(chan deliverJob, 2)
	require.True(t, offerQueuedJob(queue, incidentJob("a", "ns")).accepted)
	require.True(t, offerQueuedJob(queue, incidentJob("b", "ns")).accepted)

	result := offerQueuedJob(queue, pageAnnounce("page"))

	require.True(t, result.accepted)
	require.NotNil(t, result.displaced)
	assert.Equal(t, "b", result.displaced.key())
	assert.ElementsMatch(t, []string{"a", "page"}, queuedKeys(queue))
}

// A page never displaces a resolve or another page announcement.
func TestPageAnnounceNeverDisplacesResolveOrPage(t *testing.T) {
	queue := make(chan deliverJob, 2)
	require.True(t, offerQueuedJob(queue, pageAnnounce("p1")).accepted)
	require.True(t, offerQueuedJob(queue, resolveJob("r1")).accepted)

	result := offerQueuedJob(queue, pageAnnounce("p2"))

	assert.False(t, result.accepted)
	assert.Nil(t, result.displaced)
	assert.ElementsMatch(t, []string{"p1", "r1"}, queuedKeys(queue))
}

// With no job to displace, a page is kept in the backlog and counted;
// it is not left to the overflow summary, which pagers skip.
func TestPageAnnounceKeptInBacklogWhenQueueHasNoVictim(t *testing.T) {
	entry := providerEntry{
		provider: &fakeProvider{}, ch: make(chan deliverJob, 2),
	}
	m := managerWithEntries([]providerEntry{entry})
	entry = managerEntries(m)[0]
	entry.ch <- pageAnnounce("p1")
	entry.ch <- resolveJob("r1")
	saturated := &metrics.DefaultRegistry().DeliveryQueueSaturated
	before := saturated.Load()

	m.mu.Lock()
	m.offer(entry, pageAnnounce("p2"))
	backlog := m.backlog[entry.lookupName()]
	m.mu.Unlock()

	require.Len(t, backlog, 1)
	assert.Equal(t, "p2", backlog[0].key())
	assert.Equal(t, before+1, saturated.Load())
	assert.Empty(t, m.pacer.summaries[entry.provider.Name()])
}
