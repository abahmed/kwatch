package delivery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

// queueOnly offers jobs to a stopped worker's queue and returns them in
// queue order, as the worker would take them.
func queueOnly(
	manager *Manager, entry providerEntry, messages ...notification.Message,
) []deliverJob {
	for _, m := range messages {
		incident := m
		manager.offer(entry, deliverJob{
			kind: jobIncident, incident: &incident, target: "slack",
		})
	}
	return drainQueuedJobs(entry.ch)
}

func TestResolveOfSupersededOpeningIsSentCombined(t *testing.T) {
	cases := []struct {
		name    string
		opening *notification.Message
		want    string
	}{
		{"with the announcement attached",
			&notification.Message{Key: "a", Note: "🔴 a is down"},
			"🔴 a is down"},
		{"without it", nil, lostOpeningNote},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock()
			provider := newMessageProvider(nil)
			manager := fakeClockManager(t, clk, provider, nil)
			entry := manager.generation.entries["slack"]

			jobs := queueOnly(manager, entry,
				revision("a", 1, "🔴 a is down"),
				resolvedRevision("a", 2, "✅ a recovered", tc.opening))
			require.Len(t, jobs, 1, "the resolve replaced the opening")
			require.True(t, manager.workOne(context.Background(), &entry,
				jobs[0]))

			got := provider.receive(t)
			assert.True(t, got.Resolved())
			assert.Contains(t, got.Note, "✅ a recovered")
			assert.Contains(t, got.Note, tc.want)
			assert.Nil(t, got.Opening, "providers never see the attachment")
			assert.Zero(t, manager.openFateOf("slack", "a"),
				"the conversation is over")
		})
	}
}

func TestUpdateAfterLostOpeningAnnouncesTheConversation(t *testing.T) {
	clk := newFakeClock()
	provider := newMessageProvider(nil)
	manager := fakeClockManager(t, clk, provider, nil)
	entry := manager.generation.entries["slack"]

	jobs := queueOnly(manager, entry,
		revision("a", 1, "🔴 a is down"), revision("a", 2, "🔴 a is worse"))
	require.Len(t, jobs, 1)
	require.Equal(t, openLost, manager.openFateOf("slack", "a"))
	require.True(t, manager.workOne(context.Background(), &entry, jobs[0]))

	got := provider.receive(t)
	assert.Equal(t, "🔴 a is worse", got.Note,
		"the update already carries the whole state")
	assert.Zero(t, manager.openFateOf("slack", "a"))
}

func TestQueueAlwaysReplacesUnsentRevision(t *testing.T) {
	queue := make(chan deliverJob, 8)
	first := incidentJob("a", "ns")
	second := incidentJob("a", "ns")
	second.incident.Revision = 2

	require.True(t, offerQueuedJob(queue, first).accepted)
	result := offerQueuedJob(queue, second)

	require.NotNil(t, result.superseded, "replaced although not full")
	queued := drainQueuedJobs(queue)
	require.Len(t, queued, 1)
	assert.Equal(t, 2, queued[0].incident.Revision)
}

func TestHourlyBudgetFoldsAnnouncementsIntoTheOverflowSummary(t *testing.T) {
	clk := newFakeClock()
	provider := newMessageProvider(nil)
	manager := fakeClockManager(t, clk, provider, nil)
	entry := manager.generation.entries["slack"]
	entry.hourlyBudget = 2
	folded := metrics.DefaultRegistry().Delivery.BudgetFolded.Load()
	work := func(m notification.Message) {
		incident := m
		require.True(t, manager.workOne(context.Background(), &entry,
			deliverJob{kind: jobIncident, incident: &incident,
				target: "slack"}))
	}

	work(revision("a", 1, "🔴 a"))
	work(revision("b", 1, "🔴 b"))
	work(revision("c", 1, "🔴 c"))
	work(revision("c", 2, "🔴 c worse"))
	work(resolvedRevision("c", 3, "✅ c", nil))
	work(revision("a", 2, "🔴 a worse"))

	assert.Equal(t, 3, provider.callCount(),
		"a, b and a's update; c is summarized")
	assert.Equal(t, folded+3,
		metrics.DefaultRegistry().Delivery.BudgetFolded.Load())
	_, summary := manager.takeOverflowSummary(entry.provider.Name())
	assert.Contains(t, summary, "3 notifications were not sent one by one")
	assert.Zero(t, manager.openFateOf("slack", "c"), "c's resolve ends it")

	clk.Advance(time.Hour)
	work(revision("d", 1, "🔴 d"))
	assert.Equal(t, 4, provider.callCount(), "the budget refills")
}

func TestHeldJobNeverSkipsAQueuedResolve(t *testing.T) {
	held := deliverJob{kind: jobIncident, target: "slack",
		incident: &notification.Message{Key: "a", Revision: 1}}
	resolve := resolveJob("a")
	resolve.target = "slack"
	reopen := deliverJob{kind: jobIncident, target: "slack",
		incident: &notification.Message{Key: "a", Revision: 4}}
	queued := []deliverJob{resolve, reopen}

	index := newerRevisionIndex(queued, held)
	require.Equal(t, 0, index, "the resolve comes first")
	assert.Equal(t, -1, newerRevisionIndex(queued[1:], queued[0]),
		"a held resolve is not replaced by a reopening")
}

func TestOpenTrackingEvictsLostBeforeFolded(t *testing.T) {
	manager := &Manager{}
	manager.setOpenFate("slack", "lost", openLost)
	for i := 1; i < maxTrackedOpens; i++ {
		manager.setOpenFate("slack", fmt.Sprintf("folded-%d", i),
			openFolded)
	}
	manager.setOpenFate("slack", "extra", openFolded)

	assert.Zero(t, manager.openFateOf("slack", "lost"))
	assert.Equal(t, openFolded, manager.openFateOf("slack", "folded-1"))
	assert.Equal(t, openFolded, manager.openFateOf("slack", "extra"))
	assert.Len(t, manager.opens["slack"], maxTrackedOpens)
}

func TestOpenTrackingEvictsOldestWhenAllFolded(t *testing.T) {
	manager := &Manager{}
	for i := 0; i < maxTrackedOpens; i++ {
		manager.setOpenFate("slack", fmt.Sprintf("k-%d", i), openFolded)
	}
	manager.setOpenFate("slack", "extra", openFolded)

	assert.Zero(t, manager.openFateOf("slack", "k-0"))
	assert.Equal(t, openFolded, manager.openFateOf("slack", "k-1"))
}
