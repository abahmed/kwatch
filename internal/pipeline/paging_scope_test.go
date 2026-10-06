package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

var scopeNow = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// scopeHarness is an engine whose manager tracks an announced incident
// "a" that no paging provider has heard of, and whose sink keeps every
// message that is not carried.
func scopeHarness(t *testing.T) (*Engine, *[]notification.Message) {
	t.Helper()
	var sent []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		if m.Carrier == "" {
			sent = append(sent, m)
		}
	}
	e := newTestEngine(t, &fakeClock{now: scopeNow}, sink, nil)
	e.announcer.incidents.Restore([]incident.Record{{
		ID: "a", Root: inventory.CoreID("deployment", "shop", "a"),
		State: incident.Open, Announced: scopeNow, PagedKnown: true,
		Tier: incident.Digest,
	}}, time.Time{})
	return e, &sent
}

func resolveOf(id string, supersededBy string) incident.Decision {
	return incident.Decision{Action: incident.Resolve,
		Incident: incident.Incident{ID: id, SupersededBy: supersededBy}}
}

// A resolve reaches paging providers only when an announcement did.
func TestResolveOfNeverPagedIncidentSkipsPaging(t *testing.T) {
	e, sent := scopeHarness(t)

	e.announcer.send(context.Background(), resolveOf("a", ""), scopeNow)

	require.Len(t, *sent, 1)
	assert.True(t, (*sent)[0].SkipPaging, "chat only")
	assert.False(t, (*sent)[0].PagingOnly)
}

// The superseded close is for paging providers only; with no alert ever
// opened it reaches nobody, and the audit log still records it.
func TestSupersededResolveOfNeverPagedIncidentReachesNobody(t *testing.T) {
	e, sent := scopeHarness(t)
	var carried []notification.Message
	e.announcer.sink = func(_ context.Context, _ incident.Decision,
		m notification.Message) {
		carried = append(carried, m)
	}

	e.announcer.send(context.Background(), resolveOf("a", "b"), scopeNow)

	require.Len(t, carried, 1)
	assert.Equal(t, "unannounced", carried[0].Carrier,
		"delivery drops a carried message")
	assert.False(t, carried[0].PagingOnly)
	assert.Empty(t, *sent)
}

func TestResolveAfterAPageGoesToPagingToo(t *testing.T) {
	e, sent := scopeHarness(t)
	announce := incident.Decision{Action: incident.Announce,
		Incident: incident.Incident{ID: "a", Tier: incident.Page}}
	e.announcer.send(context.Background(), announce, scopeNow)

	e.announcer.send(context.Background(), resolveOf("a", ""), scopeNow)
	e.announcer.send(context.Background(), announce, scopeNow)
	e.announcer.send(context.Background(), resolveOf("a", "b"), scopeNow)

	require.Len(t, *sent, 4)
	assert.False(t, (*sent)[1].SkipPaging)
	assert.True(t, (*sent)[3].PagingOnly, "superseded stays paging-only")
	assert.False(t, (*sent)[3].SkipPaging)
}

// A resolve closes the alert at the paging providers, so the incident is
// no longer paged: a second resolve (a reopen that recovered before its
// "failing again" update) must not close the closed alert again, and the
// update that follows a reopen opens it again.
func TestPagedFollowsTheAlertAcrossResolveAndReopen(t *testing.T) {
	e, sent := scopeHarness(t)
	ctx := context.Background()
	announce := incident.Decision{Action: incident.Announce,
		Incident: incident.Incident{ID: "a", Tier: incident.Page}}
	e.announcer.send(ctx, announce, scopeNow)
	assert.True(t, e.announcer.incidents.Paged("a"))

	e.announcer.send(ctx, resolveOf("a", ""), scopeNow)
	assert.False(t, e.announcer.incidents.Paged("a"), "alert closed")
	e.announcer.send(ctx, resolveOf("a", ""), scopeNow)
	assert.True(t, (*sent)[2].SkipPaging, "nothing left to close")

	again := incident.Decision{Action: incident.Update,
		Incident: incident.Incident{ID: "a", Tier: incident.Notify}}
	e.announcer.send(ctx, again, scopeNow)
	assert.True(t, e.announcer.incidents.Paged("a"), "alert open again")
}

func TestPageHeldInSummaryIsRememberedAsPaged(t *testing.T) {
	e, sent := scopeHarness(t)
	e.announcer.collect.Startup.Until = scopeNow.Add(time.Minute)
	page := incident.Decision{Action: incident.Announce,
		Incident: incident.Incident{ID: "a", Tier: incident.Page,
			Opened: scopeNow.Add(-time.Hour)}}

	e.announcer.collect.CollectStartup(context.Background(), scopeNow,
		[]incident.Decision{page})
	require.Len(t, *sent, 1, "the page goes on its own")
	assert.True(t, e.announcer.incidents.Paged("a"))
}

func TestAuditEntryRecordsChatOnlyResolve(t *testing.T) {
	d := resolveOf("a", "")
	entry := AuditEntry(d, notification.Message{SkipPaging: true}, scopeNow)
	assert.Equal(t, "chat", entry.Delivery)
}

// A digest's audit entry carries counts and the first incidents named.
func TestDigestAuditEntryCarriesContent(t *testing.T) {
	e, sent := digestHarness(t, scopeNow)
	var decisions []incident.Decision
	for i := 0; i < announce.MaxAuditItems+5; i++ {
		decisions = append(decisions, digestAnnounce(fmt.Sprintf("i%02d", i)))
	}
	resolved := digestAnnounce("old")
	resolved.Action = incident.Resolve

	e.announcer.collect.CollectDigest(context.Background(), scopeNow, decisions)
	e.announcer.collect.CollectDigest(context.Background(), scopeNow,
		[]incident.Decision{resolved})
	e.announcer.collect.CollectDigest(context.Background(),
		scopeNow.Add(announce.DigestWindow), nil)

	require.Len(t, *sent, 1)
	listed := (*sent)[0].Listed
	require.NotNil(t, listed)
	assert.Equal(t, announce.MaxAuditItems+5, listed.Opened)
	assert.Equal(t, 1, listed.Resolved)
	assert.Len(t, listed.Items, announce.MaxAuditItems, "bounded")
	assert.Contains(t, listed.Items[0], "i00: ")

	entry := AuditEntry(incident.Decision{Reason: "digest"}, (*sent)[0],
		scopeNow)
	assert.Equal(t, announce.MaxAuditItems+5, entry.Opened)
	assert.Equal(t, 1, entry.Resolved)
	assert.Equal(t, listed.Items, entry.Items)
}

func digestRecurrence(
	id string, history ...incident.Occurrence,
) incident.Decision {
	d := digestAnnounce(id)
	d.Incident.History = history
	return d
}

// A recurrence that resolves before its digest still shows up in it; a
// first occurrence stays out.
func TestDigestListsRecurrenceResolvedBeforeFlush(t *testing.T) {
	e, sent := digestHarness(t, scopeNow)
	again := digestRecurrence("again", incident.Occurrence{Heard: true})
	blip := digestRecurrence("blip")
	ctx := context.Background()

	e.announcer.collect.CollectDigest(ctx, scopeNow,
		[]incident.Decision{again, blip})
	resolve := func(d incident.Decision) incident.Decision {
		d.Action = incident.Resolve
		return d
	}
	e.announcer.collect.CollectDigest(ctx, scopeNow,
		[]incident.Decision{resolve(again), resolve(blip)})

	g := e.announcer.collect.Low
	require.Len(t, g.Resolved, 1)
	assert.Equal(t, "again", g.Resolved[0].Incident.ID)
	assert.Empty(t, g.Opened)

	e.announcer.collect.CollectDigest(
		ctx, scopeNow.Add(announce.DigestWindow), nil)
	require.Len(t, *sent, 1)
	assert.Equal(t, 1, (*sent)[0].Listed.Resolved)
}

// A reminder of a long-open digest incident is listed in the next digest,
// with how long it has failed, and resolving it later is told.
func TestDigestListsChronicReminder(t *testing.T) {
	e, sent := digestHarness(t, scopeNow)
	ctx := context.Background()
	reminder := digestAnnounce("old")
	reminder.Action, reminder.Reason = incident.Update, incident.ReasonReminder
	reminder.Incident.Opened = scopeNow.Add(-72 * time.Hour)

	e.announcer.collect.CollectDigest(ctx, scopeNow, []incident.Decision{reminder})
	require.Len(t, e.announcer.collect.Low.Opened, 1)
	resolve := reminder
	resolve.Action = incident.Resolve
	e.announcer.collect.CollectDigest(ctx, scopeNow, []incident.Decision{resolve})
	assert.Empty(t, e.announcer.collect.Low.Opened)
	assert.Len(t, e.announcer.collect.Low.Resolved, 1,
		"it was told, so say it ended")

	e.announcer.collect.CollectDigest(
		ctx, scopeNow.Add(announce.DigestWindow), nil)
	require.Len(t, *sent, 1)
}

func TestDigestReminderIsListedWithDuration(t *testing.T) {
	e, sent := digestHarness(t, scopeNow)
	reminder := digestAnnounce("old")
	reminder.Action, reminder.Reason = incident.Update, incident.ReasonReminder
	reminder.Incident.Opened = scopeNow.Add(-72 * time.Hour)

	e.announcer.collect.CollectDigest(context.Background(), scopeNow,
		[]incident.Decision{reminder})
	e.announcer.collect.CollectDigest(context.Background(),
		scopeNow.Add(announce.DigestWindow), nil)

	require.Len(t, *sent, 1)
	assert.Contains(t, (*sent)[0].Note, "three days")
}
