package delivery

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/metrics"
)

// More jobs wait for a provider at Start than its queue holds: the rest
// go to the backlog, not to the dead letters and the overflow summary.
func TestReplayOverflowGoesToBacklogNotDeadLetters(t *testing.T) {
	entry := providerEntry{provider: &fakeProvider{},
		ch: make(chan deliverJob, channelCap)}
	m := managerWithEntries([]providerEntry{entry})
	entry = managerEntries(m)[0]
	name := entry.lookupName()
	const restored, pending = 200, 100
	for i := range restored {
		job := incidentJob(fmt.Sprintf("old-%d", i), "ns")
		job.target = name
		m.backlog = map[string][]deliverJob{
			name: append(m.backlog[name], job)}
	}
	for i := range pending {
		job := incidentJob(fmt.Sprintf("new-%d", i), "ns")
		job.target = name
		m.pending = append(m.pending, job)
	}
	dead := &metrics.DefaultRegistry().DeliveryDeadLetters
	before := dead.Load()

	m.mu.Lock()
	m.replayQueuedLocked()
	waiting := len(m.backlog[name])
	m.mu.Unlock()

	require.Equal(t, restored+pending, len(entry.ch)+waiting,
		"every job is queued or waits in the backlog")
	assert.Positive(t, waiting)
	assert.Equal(t, before, dead.Load(), "nothing is dead-lettered")
	assert.False(t, m.hasOverflowSummary(entry), "nothing is summarized")
}
