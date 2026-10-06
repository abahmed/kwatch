package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

var holdsNow = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// holdsHarness is an engine whose manager tracks the announced incidents
// of namespace shop with the given IDs, and a sink that keeps every
// message that reaches delivery (not the carried ones).
func holdsHarness(
	t *testing.T, tier incident.Tier, ids ...string,
) (*Engine, *[]notification.Message) {
	t.Helper()
	var seen []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		if m.Carrier == "" {
			seen = append(seen, m)
		}
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	var records []incident.Record
	for i, id := range ids {
		records = append(records, incident.Record{
			ID: id, Tier: tier, State: incident.Open,
			Root: inventory.EntityID{Kind: kube.KindDeployment,
				Namespace: "shop", Name: id + string(rune('a'+i))},
			Opened: holdsNow.Add(-time.Minute), Announced: holdsNow,
			PagedKnown: true,
		})
	}
	e.deps.Incidents.Restore(records, time.Time{})
	return e, &seen
}

// decisionOf returns the manager's current view of incident id as a
// decision of the given action, with its tier kept.
func decisionOf(
	e *Engine, id string, action incident.Action,
) incident.Decision {
	for _, p := range e.deps.Incidents.Incidents() {
		if p.ID == id {
			return incident.Decision{Action: action, Incident: p}
		}
	}
	return incident.Decision{Action: action}
}

func heldOf(e *Engine, id string) bool {
	for _, r := range e.deps.Incidents.Export() {
		if r.ID == id {
			return r.Held
		}
	}
	return false
}

// A page-tier incident collected by the startup summary is paged on its
// own; if it resolves inside the window, its paging alert is closed too.
func TestStartupHeldPageResolveClosesThePager(t *testing.T) {
	e, seen := holdsHarness(t, incident.Page, "p")
	e.announcer.collect.Startup.Until = holdsNow.Add(time.Minute)
	ctx := context.Background()
	e.announcer.collect.CollectStartup(ctx, holdsNow,
		[]incident.Decision{decisionOf(e, "p", incident.Announce)})
	require.Len(t, *seen, 1)
	require.True(t, (*seen)[0].PagingOnly, "the page went out alone")
	require.True(t, e.deps.Incidents.Paged("p"))

	e.announcer.collect.CollectStartup(ctx, holdsNow.Add(10*time.Second),
		[]incident.Decision{decisionOf(e, "p", incident.Resolve)})

	require.Len(t, *seen, 2, "the resolve must reach the pagers")
	assert.True(t, (*seen)[1].PagingOnly)
	assert.False(t, e.deps.Incidents.Paged("p"), "the alert is closed")
	assert.False(t, heldOf(e, "p"))
}

// A held incident that never paged resolves without any message.
func TestStartupHeldNonPageResolveSendsNothing(t *testing.T) {
	e, seen := holdsHarness(t, incident.Notify, "n")
	e.announcer.collect.Startup.Until = holdsNow.Add(time.Minute)
	ctx := context.Background()
	e.deps.Incidents.RecordPaged("n", false)
	e.announcer.collect.CollectStartup(ctx, holdsNow,
		[]incident.Decision{decisionOf(e, "n", incident.Announce)})
	e.announcer.collect.CollectStartup(ctx, holdsNow.Add(10*time.Second),
		[]incident.Decision{decisionOf(e, "n", incident.Resolve)})

	assert.Empty(t, *seen)
	assert.False(t, heldOf(e, "n"))
}

// An outage hold that turns out not to be an outage hands its
// announcements back to normal delivery; once delivery has them they are
// no longer held, or a restart would announce them again.
func TestReleasedOutageAnnouncementIsNoLongerHeld(t *testing.T) {
	e, seen := holdsHarness(t, incident.Notify, "n")
	e.announcer.collect.Outages["shop"] = &announce.OutageHold{Since: holdsNow}

	e.announcer.announce(context.Background(), holdsNow,
		[]incident.Decision{decisionOf(e, "n", incident.Announce)})

	require.Len(t, *seen, 1, "announced on its own")
	assert.False(t, heldOf(e, "n"), "delivery has it, so it is released")
}

// The first page-tier member of an outage hold pages at once. If the
// hold is released as not an outage, that same incident is announced to
// chat only: the pagers already have it.
func TestReleasedOutagePageIsNotPagedTwice(t *testing.T) {
	e, seen := holdsHarness(t, incident.Page, "p")
	e.announcer.collect.Outages["shop"] = &announce.OutageHold{Since: holdsNow}

	e.announcer.announce(context.Background(), holdsNow,
		[]incident.Decision{decisionOf(e, "p", incident.Announce)})

	require.Len(t, *seen, 2, "one page, one chat announcement")
	assert.True(t, (*seen)[0].PagingOnly)
	assert.False(t, (*seen)[1].PagingOnly)
	assert.True(t, (*seen)[1].SkipPaging,
		"the released announcement must not reach the pagers again")
}
