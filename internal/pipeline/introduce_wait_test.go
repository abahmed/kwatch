package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// An introduction goes to chat only, so it opens nothing on the pagers:
// a later resolve must not close an alert nobody opened.
func TestIntroductionOpensNoPage(t *testing.T) {
	r := newIntroRig(t)
	r.rollUp(t)
	before := r.e.deps.Incidents.PageOpen("a")
	r.investigate(t, "a")
	r.e.announcer.announce(context.Background(),
		holdsNow.Add(time.Second), nil)
	require.Len(t, r.introductions(), 1)
	assert.Equal(t, before, r.e.deps.Incidents.PageOpen("a"),
		"the introduction changed what the pagers hold")
}

// A namespace outage holds its members before its listing goes out:
// an introduction whose evidence is in waits for that listing, then
// goes out once, instead of being dropped.
func TestIntroductionWaitsForAHeldListing(t *testing.T) {
	r := newIntroRig(t)
	r.e.announcer.noteListed([]incident.Decision{
		decisionOf(r.e, "a", incident.Announce)}, nil, holdsNow)
	r.investigate(t, "a")

	r.e.announcer.announce(context.Background(),
		holdsNow.Add(time.Second), nil)
	require.Empty(t, r.introductions(), "the listing is still held")
	require.Contains(t, r.e.announcer.intros, "a", "kept waiting")

	r.e.deps.Incidents.RecordRolledUp("a")
	r.e.announcer.announce(context.Background(),
		holdsNow.Add(2*time.Second), nil)
	r.e.announcer.announce(context.Background(),
		holdsNow.Add(time.Minute), nil)
	intros := r.introductions()
	require.Len(t, intros, 1, "introduced once the listing went out")
	assert.Equal(t, "a", r.sent[intros[0]].Incident.ID)
}

// The wait for a held listing is bounded: past outputWait the entry
// goes, introduced or not.
func TestIntroductionWaitForAListingIsBounded(t *testing.T) {
	r := newIntroRig(t)
	r.e.announcer.noteListed([]incident.Decision{
		decisionOf(r.e, "a", incident.Announce)}, nil, holdsNow)
	r.investigate(t, "a")
	r.e.announcer.announce(context.Background(),
		holdsNow.Add(outputWait+time.Second), nil)
	assert.Empty(t, r.introductions())
	assert.Empty(t, r.e.announcer.intros)
}
