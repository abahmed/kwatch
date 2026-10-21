package announce

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// A problem the restored-incidents summary holds may rise to the page
// tier while it waits. The summary is chat only, so the page must go to
// the pagers at once: folded into the summary it would page nobody until
// the problem resolved.
func TestRestoredSummaryPagesAHeldProblemThatRisesToPage(t *testing.T) {
	c, sent := foldHarness()
	c.Startup.WarmAt = foldNow.Add(10 * time.Minute)
	ctx := context.Background()
	held := oldFinding(foldNow.Add(-300 * 24 * time.Hour))
	c.CollectStartup(ctx, foldNow, []incident.Decision{held})
	assert.Empty(t, *sent, "setup: the summary holds the announcement")

	rise := held
	rise.Action, rise.Reason = incident.Update, incident.ReasonMaterialChange
	rise.Incident.Tier = incident.Page
	rest, _ := c.CollectStartup(ctx, foldNow.Add(time.Minute),
		[]incident.Decision{rise})

	assert.Empty(t, rest, "the summary still owns the problem")
	assert.Equal(t, 1, pagesIn(*sent), "the page is not delayed")
}

// A rise that was already paged is not paged twice.
func TestRestoredSummaryDoesNotPageAnAlreadyPagedProblemAgain(t *testing.T) {
	known := oldFinding(foldNow)
	c, sent := foldHarness(incident.Record{ID: known.Incident.ID,
		Root: known.Incident.Root, Tier: incident.Notify,
		State: incident.Open, Opened: foldNow, Announced: foldNow,
		PagedKnown: true})
	c.Startup.WarmAt = foldNow.Add(10 * time.Minute)
	ctx := context.Background()
	held := oldFinding(foldNow.Add(-300 * 24 * time.Hour))
	c.CollectStartup(ctx, foldNow, []incident.Decision{held})
	rise := held
	rise.Action, rise.Reason = incident.Update, incident.ReasonMaterialChange
	rise.Incident.Tier = incident.Page
	c.CollectStartup(ctx, foldNow.Add(time.Minute), []incident.Decision{rise})

	again := rise
	again.Reason = incident.ReasonReminder
	c.CollectStartup(ctx, foldNow.Add(2*time.Minute),
		[]incident.Decision{again})

	assert.Equal(t, 1, pagesIn(*sent))
}
