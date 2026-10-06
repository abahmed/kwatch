package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// Releasing a held announcement changes what the incident record says
// (it is no longer held), so the loop is told to save it promptly.
func TestExpireHeldAndAttachOutputReportTheyReleasedOne(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	e := investigatingEngine(t, clock, g, &sinkLog{})
	startPool(t, e)
	ctx := context.Background()

	e.announcer.deliver(ctx, clock.now, []incident.Decision{announcement("a")})
	assert.False(t, e.announcer.expireHeld(ctx, clock.now),
		"nothing is due yet")
	assert.True(t, e.announcer.expireHeld(ctx, clock.now.Add(outputWait)),
		"the wait ended and the announcement went")
	close(g.release)
	assert.False(t, e.announcer.attachOutput(ctx, receive(t, e)),
		"a result for an announcement that already went releases nothing")

	e.announcer.deliver(ctx, clock.now, []incident.Decision{announcement("b")})
	r := receive(t, e)
	assert.True(t, e.announcer.attachOutput(ctx, r), "released by its result")
}

// The cold-start summary closes at the end of its window, not at the next
// heartbeat: the window end is a wake-up deadline.
func TestStartupWindowEndIsAWakeDeadline(t *testing.T) {
	e := newTestEngine(t, &fakeClock{now: holdsNow}, (&sinkLog{}).sink, nil)
	assert.True(t, e.announcer.collect.NextStartup().IsZero())

	e.announcer.collect.Startup.Until = holdsNow.Add(2 * time.Second)
	assert.Equal(t, holdsNow.Add(2*time.Second),
		e.announcer.collect.NextStartup())
}

// The fingerprints cover every tracked object; they are built only when
// the store would write them, and always for the final save.
func TestFingerprintsAreBuiltOnlyWhenTheStoreWillWriteThem(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Store = &memStore{} })
	e.storage.reconciled = true

	e.saveAt(now, false)
	assert.Equal(t, now, e.storage.fingerprinted, "the first is due")
	e.saveAt(now.Add(time.Minute), false)
	assert.Equal(t, now, e.storage.fingerprinted, "too soon to be written")
	e.saveAt(now.Add(fingerprintInterval), false)
	assert.Equal(t, now.Add(fingerprintInterval), e.storage.fingerprinted)
	e.saveAt(now.Add(fingerprintInterval+time.Second), true)
	assert.Equal(t, now.Add(fingerprintInterval+time.Second),
		e.storage.fingerprinted, "the final save forces them")
}

// An open incident keeps its evidence however old: forgetting it would
// make the next update quote every fact again as new.
func TestEvidenceOfAnOpenIncidentIsKept(t *testing.T) {
	e, _ := holdsHarness(t, incident.Notify, "open")
	now := holdsNow
	old := now.Add(-2 * evidenceMemory)
	e.announcer.evidence["open"] = &evidenceState{started: old}
	e.announcer.evidence["gone"] = &evidenceState{started: old}

	e.announcer.forgetOldEvidence(now)

	assert.Contains(t, e.announcer.evidence, "open")
	assert.NotContains(t, e.announcer.evidence, "gone",
		"an incident the manager no longer tracks is forgotten")
}

// The digest lines that no incident record keeps (a reminder, a return,
// a resolve of a listed incident) survive a restart.
func TestPendingDigestLinesSurviveARestart(t *testing.T) {
	e, _ := holdsHarness(t, incident.Digest, "a", "b")
	c := e.announcer.collect
	remind := decisionOf(e, "a", incident.Update)
	remind.Reason = incident.ReasonReminder
	resolved := decisionOf(e, "b", incident.Resolve)
	c.Low.Opened = []incident.Decision{remind}
	c.Low.Resolved = []incident.Decision{resolved}
	c.Low.Since = holdsNow

	c.CollectDigest(context.Background(), holdsNow.Add(time.Minute), nil)
	saved := c.Startup.Summary
	require.Len(t, saved.Digest.Told, 1)
	require.Len(t, saved.Digest.Resolved, 1)

	fresh, _ := holdsHarness(t, incident.Digest, "a", "b")
	fresh.announcer.collect.Startup.Summary = saved
	fresh.announcer.collect.RestoreDigest()

	low := fresh.announcer.collect.Low
	require.Len(t, low.Opened, 1)
	assert.Equal(t, incident.ReasonReminder, low.Opened[0].Reason)
	assert.Equal(t, "a", low.Opened[0].Incident.ID)
	require.Len(t, low.Resolved, 1)
	assert.Equal(t, "b", low.Resolved[0].Incident.ID)
	assert.Equal(t, holdsNow, low.Since)
}

// A digest-listed incident that later rises out of the digest tier is
// introduced in full by its first own message, once.
func TestDigestListedIncidentIsAnnouncedWhenItEscalates(t *testing.T) {
	e, _ := holdsHarness(t, incident.Digest, "a")
	c := e.announcer.collect
	ctx := context.Background()
	c.CollectDigest(ctx, holdsNow, []incident.Decision{
		decisionOf(e, "a", incident.Announce)})
	c.CollectDigest(ctx, holdsNow.Add(announce.DigestWindow), nil)

	up := decisionOf(e, "a", incident.Update)
	up.Incident.Tier = incident.Notify
	rest, _ := c.CollectDigest(ctx, holdsNow.Add(time.Hour),
		[]incident.Decision{up})
	require.Len(t, rest, 1)
	assert.Equal(t, incident.Announce, rest[0].Action,
		"the digest only named it")

	rest, _ = c.CollectDigest(ctx, holdsNow.Add(time.Hour),
		[]incident.Decision{up})
	assert.Equal(t, incident.Update, rest[0].Action, "then it is an update")
}

// A digest-tier update with nothing to list is audited as dropped, not
// as carried by a digest that never mentions it.
func TestDroppedDigestUpdateIsAuditedAsDropped(t *testing.T) {
	var carriers, notes []string
	sink := func(_ context.Context, _ incident.Decision,
		m notification.Message) {
		carriers = append(carriers, m.Carrier)
		notes = append(notes, m.CarrierNote)
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	up := incident.Decision{Action: incident.Update,
		Reason:   incident.ReasonMaterialChange,
		Incident: incident.Incident{ID: "x", Tier: incident.Digest}}

	rest, _ := e.announcer.collect.CollectDigest(context.Background(),
		holdsNow, []incident.Decision{up})

	assert.Empty(t, rest)
	assert.Equal(t, []string{announce.CarrierDropped}, carriers)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "digest-only")
	assert.Contains(t, notes[0], string(incident.ReasonMaterialChange))
}

// The resolve of a blip nobody was told about is dropped on purpose, and
// the audit entry says so.
func TestDroppedDigestResolveSaysWhy(t *testing.T) {
	var notes []string
	sink := func(_ context.Context, _ incident.Decision,
		m notification.Message) {
		notes = append(notes, m.CarrierNote)
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	in := incident.Incident{ID: "x", Tier: incident.Digest}
	c := e.announcer.collect
	c.CollectDigest(context.Background(), holdsNow, []incident.Decision{
		{Action: incident.Announce, Incident: in}})
	notes = nil

	c.CollectDigest(context.Background(), holdsNow, []incident.Decision{
		{Action: incident.Resolve, Incident: in}})

	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "blip nobody was told about")
}

// A listing counts a member as having said it resolved only when the
// resolve reached chat. A resolve that went to the pagers alone leaves
// the roll-up to close with its own message; one chat got does not.
func TestRollupClosesWhenAMembersResolveDidNotReachChat(t *testing.T) {
	for name, unannounced := range map[string]bool{
		"pagers only": true, "chat": false} {
		t.Run(name, func(t *testing.T) {
			var reasons []incident.Reason
			sink := func(_ context.Context, d incident.Decision,
				_ notification.Message) {
				reasons = append(reasons, d.Reason)
			}
			e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
			c := e.announcer.collect
			c.Startup.Summary.Rollups = []announce.Listing{{
				Key: "rollup/x", Incidents: []string{"a"}}}
			resolve := incident.Decision{Action: incident.Resolve,
				Reason: "healthy", Unannounced: unannounced,
				Incident: incident.Incident{ID: "a"}}

			e.announcer.send(context.Background(), resolve, holdsNow)
			c.Startup.CheckSummary = true
			c.CloseListings(context.Background())

			closed := false
			for _, r := range reasons {
				closed = closed || r == "roll-up resolved"
			}
			assert.Equal(t, unannounced, closed)
		})
	}
}

// A run that stopped inside its startup window saved an incomplete
// marker with the roll-ups it had sent. They are still owed their
// resolve, so a cold start keeps them.
func TestColdStartKeepsTheRollupsOfAnIncompleteMarker(t *testing.T) {
	marker := announce.StartupState{Rollups: []announce.Listing{{
		Key: "rollup/x", Incidents: []string{"a"}}}}
	var s announce.Startup

	s.Restore(marker, true, 1, holdsNow)

	assert.True(t, s.ColdStart, "the window had not completed")
	assert.Len(t, s.Summary.Rollups, 1)
	assert.True(t, s.CheckSummary)
}
