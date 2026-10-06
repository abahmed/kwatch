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

// A Notify incident held in the startup summary that rises to Page
// before the window ends must page: the summary never reaches pagers.
func TestStartupHeldIncidentThatRisesToPagePages(t *testing.T) {
	e, seen := holdsHarness(t, incident.Notify, "n")
	e.announcer.collect.Startup.Until = holdsNow.Add(time.Minute)
	ctx := context.Background()
	e.announcer.collect.CollectStartup(ctx, holdsNow,
		[]incident.Decision{decisionOf(e, "n", incident.Announce)})
	require.Empty(t, *seen, "a notify is carried by the summary")

	up := decisionOf(e, "n", incident.Update)
	up.Incident.Tier = incident.Page
	e.announcer.collect.CollectStartup(ctx, holdsNow.Add(10*time.Second),
		[]incident.Decision{up})

	require.Equal(t, 1, pagingOnly(*seen), "the pagers were told")
	assert.True(t, e.deps.Incidents.Paged("n"), "so the resolve follows")

	again := decisionOf(e, "n", incident.Update)
	again.Incident.Tier = incident.Page
	e.announcer.collect.CollectStartup(ctx, holdsNow.Add(20*time.Second),
		[]incident.Decision{again})
	assert.Equal(t, 1, pagingOnly(*seen), "one page, not one per update")
}

// The same rise inside a namespace outage hold pages once for the
// namespace: the first page-tier member stands for it.
func TestOutageHeldIncidentThatRisesToPagePagesOnce(t *testing.T) {
	e, seen := holdsHarness(t, incident.Notify, "n", "m")
	hold := &announce.OutageHold{Since: holdsNow}
	e.announcer.collect.Outages["shop"] = hold
	ctx := context.Background()
	for _, id := range []string{"n", "m"} {
		e.announcer.collect.HoldOutage(ctx, holdsNow,
			decisionOf(e, id, incident.Announce))
	}
	require.Empty(t, *seen)

	for _, id := range []string{"n", "m"} {
		up := decisionOf(e, id, incident.Update)
		up.Incident.Tier = incident.Page
		e.announcer.collect.HoldOutage(ctx, holdsNow.Add(time.Second), up)
	}

	require.Equal(t, 1, pagingOnly(*seen), "the namespace pages once")
	assert.True(t, hold.Paged)
	assert.Equal(t, "n", hold.PagedID)
}

// An outage hold that pages its first page-tier member and then does not
// qualify as an outage releases its announcements; the roll-up of the
// same tick must not page that incident a second time.
func TestOutageReleaseThenRollupPagesOnce(t *testing.T) {
	var seen []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		seen = append(seen, m)
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	mk := func(id string, tier incident.Tier, n string) incident.Record {
		return incident.Record{ID: id, Tier: tier, State: incident.Open,
			Root: inventory.EntityID{Kind: kube.KindDeployment,
				Namespace: "shop", Name: n},
			Opened: holdsNow.Add(-time.Minute), Announced: holdsNow,
			PagedKnown: true}
	}
	e.deps.Incidents.Restore([]incident.Record{
		mk("p", incident.Page, "pa"), mk("q", incident.Notify, "qa")},
		time.Time{})
	e.deps.Incidents.RecordPaged("p", false)
	e.announcer.collect.Outages["shop"] = &announce.OutageHold{Since: holdsNow}

	e.announcer.announce(context.Background(), holdsNow, []incident.Decision{
		decisionOf(e, "p", incident.Announce),
		decisionOf(e, "q", incident.Announce)})

	var pages int
	for _, m := range seen {
		if m.PagingOnly && m.Carrier == "" {
			pages++
		}
	}
	assert.Equal(t, 1, pages, "one page for p, however it was released")
}

// A page-held incident that comes back is chat news: the update does not
// go to the pagers and does not open an alert again.
func TestFailingAgainOfAHeldPageStaysInChat(t *testing.T) {
	var seen []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		seen = append(seen, m)
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	e.deps.Incidents.Restore([]incident.Record{{
		ID: "a", Tier: incident.Notify, State: incident.Open,
		Root: inventory.EntityID{Kind: kube.KindDeployment,
			Namespace: "shop", Name: "a"},
		Opened: holdsNow, Announced: holdsNow, PagedKnown: true,
		PageHeld: true,
	}}, time.Time{})
	require.False(t, e.deps.Incidents.Paged("a"))

	d := decisionOf(e, "a", incident.Update)
	d.Reason = incident.ReasonFailingAgain
	e.announcer.send(context.Background(), d, holdsNow)

	require.Len(t, seen, 1)
	assert.True(t, seen[0].SkipPaging, "chat only")
	assert.False(t, e.deps.Incidents.Paged("a"), "the page stays closed")
}

// The resolve of an incident that paged but was never announced closes
// the pager alert and goes to the pagers alone.
func TestUnannouncedResolveIsPagingOnly(t *testing.T) {
	var seen []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		seen = append(seen, m)
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	e.deps.Incidents.Restore([]incident.Record{{
		ID: "a", Tier: incident.Page, State: incident.Open,
		Root: inventory.EntityID{Kind: kube.KindDeployment,
			Namespace: "shop", Name: "a"},
		Opened: holdsNow, Announced: holdsNow, PagedKnown: true,
		Paged: true,
	}}, time.Time{})

	d := decisionOf(e, "a", incident.Resolve)
	d.Unannounced = true
	e.announcer.send(context.Background(), d, holdsNow)

	require.Len(t, seen, 1)
	assert.True(t, seen[0].PagingOnly)
	assert.False(t, e.deps.Incidents.Paged("a"), "the alert is closed")
}

// Held announcements of incidents that resolved quietly are purged from
// every collector, and the listings are checked.
func TestQuietResolvePurgesHeldAnnouncements(t *testing.T) {
	e, seen := holdsHarness(t, incident.Notify, "a", "b")
	c := e.announcer.collect
	c.Startup.Collected = []incident.Decision{
		decisionOf(e, "a", incident.Announce),
		decisionOf(e, "b", incident.Announce)}
	c.Low.Opened = []incident.Decision{decisionOf(e, "a", incident.Announce)}
	c.Outages["shop"] = &announce.OutageHold{Since: holdsNow,
		Held: []incident.Decision{decisionOf(e, "a", incident.Announce)}}
	e.announcer.held = []heldAnnouncement{
		{decision: decisionOf(e, "a", incident.Announce)},
		{decision: decisionOf(e, "b", incident.Announce)}}

	e.announcer.dropHeld([]string{"a"})
	c.NoteQuietResolves([]string{"a"})

	require.Len(t, e.announcer.held, 1)
	assert.Equal(t, "b", e.announcer.held[0].decision.Incident.ID)
	require.Len(t, c.Startup.Collected, 1)
	assert.Equal(t, "b", c.Startup.Collected[0].Incident.ID)
	assert.Empty(t, c.Low.Opened)
	assert.Empty(t, c.Outages["shop"].Held)
	assert.True(t, c.Startup.CheckSummary)
	assert.Empty(t, *seen)
}

// Page once per outage: after a reopen held at notify, every later
// message is chat only, until the tier rises to Page again.
func TestHeldPageStaysInChatForEveryLaterMessage(t *testing.T) {
	var seen []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		seen = append(seen, m)
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	e.deps.Incidents.Restore([]incident.Record{{
		ID: "a", Tier: incident.Notify, State: incident.Open,
		Root: inventory.EntityID{Kind: kube.KindDeployment,
			Namespace: "shop", Name: "a"},
		Opened: holdsNow, Announced: holdsNow, PagedKnown: true,
		PageHeld: true,
	}}, time.Time{})

	for _, why := range []incident.Reason{
		incident.ReasonFailingAgain, incident.ReasonReminder,
		incident.ReasonMaterialChange} {
		d := decisionOf(e, "a", incident.Update)
		d.Reason = why
		e.announcer.send(context.Background(), d, holdsNow)
	}
	require.Len(t, seen, 3)
	for _, m := range seen {
		assert.True(t, m.SkipPaging, "chat only")
	}
	assert.False(t, e.deps.Incidents.Paged("a"), "no alert was opened")

	rise := decisionOf(e, "a", incident.Update)
	rise.Incident.Tier = incident.Page
	e.announcer.send(context.Background(), rise, holdsNow)
	assert.False(t, seen[3].SkipPaging, "a real rise pages")
	assert.True(t, e.deps.Incidents.Paged("a"))
}
