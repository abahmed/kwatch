package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

func TestOfferQueuesBehindBacklogResolveOfSamePage(t *testing.T) {
	provider := newMessageProvider(nil)
	manager := fakeClockManager(t, newFakeClock(), provider, nil)
	entry := manager.generation.entries["slack"]

	resolveA := resolvedRevision("a", 2, "a recovered", nil)
	resolveA.DedupKey = "page"
	triggerB := revision("b", 1, "b failed")
	triggerB.DedupKey = "page"
	unrelated := revision("c", 1, "c failed")

	manager.mu.Lock()
	manager.toBacklogLocked(deliverJob{
		kind: jobIncident, incident: &resolveA, target: "slack",
	})
	manager.offer(entry, deliverJob{
		kind: jobIncident, incident: &triggerB, target: "slack",
	})
	manager.offer(entry, deliverJob{
		kind: jobIncident, incident: &unrelated, target: "slack",
	})
	manager.mu.Unlock()

	// The channel got the unrelated job first or the backlog refilled
	// in order; either way a resolve precedes its page's next trigger.
	var order []notification.Message
	for _, job := range drainQueuedJobs(entry.ch) {
		order = append(order, *job.incident)
	}
	require.Len(t, order, 3)
	pos := map[string]int{}
	for i, m := range order {
		pos[m.Key] = i
	}
	assert.Less(t, pos["a"], pos["b"])
	assert.Empty(t, manager.backlog["slack"])
}
