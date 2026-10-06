package delivery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

func pageRevision(key string, rev int) notification.Message {
	m := revision(key, rev, "🔴 "+key)
	m.Route.Severity = "critical"
	return m
}

func budgetWorker(
	t *testing.T, budget int,
) (*Manager, *messageProvider, func(notification.Message)) {
	t.Helper()
	provider := newMessageProvider(nil)
	manager := fakeClockManager(t, newFakeClock(), provider, nil)
	entry := manager.generation.entries["slack"]
	entry.hourlyBudget = budget
	work := func(m notification.Message) {
		incident := m
		require.True(t, manager.workOne(context.Background(), &entry,
			deliverJob{kind: jobIncident, incident: &incident,
				target: "slack"}))
	}
	return manager, provider, work
}

// A page announcement is never folded into an overflow summary: a pager
// skips the summary, so the page would be lost for good.
func TestHourlyBudgetNeverFoldsPageAnnouncements(t *testing.T) {
	manager, provider, work := budgetWorker(t, 1)

	work(revision("a", 1, "🟠 a"))
	work(pageRevision("p1", 1))
	work(pageRevision("p2", 1))
	work(resolvedRevision("p2", 2, "✅ p2", nil))

	assert.Equal(t, 4, provider.callCount(),
		"both pages and the resolve of a paged incident are sent")
	_, summary := manager.takeOverflowSummary("slack")
	assert.Empty(t, summary)
}

// An incident folded at notify tier that later reaches the page tier is
// sent as an announcement, because the pager never saw it.
func TestHourlyBudgetLetsEscalationToPageThrough(t *testing.T) {
	manager, provider, work := budgetWorker(t, 1)

	work(revision("a", 1, "🟠 a"))
	work(revision("b", 1, "🟠 b"))
	require.Equal(t, 1, provider.callCount(), "b is folded")
	work(pageRevision("b", 2))

	require.Equal(t, 2, provider.callCount())
	assert.Zero(t, manager.openFateOf("slack", "b"),
		"the page was sent, so nothing is folded any more")
}
