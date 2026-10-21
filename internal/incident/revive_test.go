package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A probe that fails for a few seconds, over and over, is one incident
// that has not been told yet, not a new incident each time: the ID stays.
func TestFlickeringFindingKeepsOneIncidentID(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	id := r.idOf(web.Entity)
	now := at(0)

	for range 6 {
		now = now.Add(10 * time.Second)
		r.clear(now, web)
		wantNone(t, r.tick(now))
		now = now.Add(10 * time.Second)
		r.raise(now, web)
		wantNone(t, r.tick(now))
	}

	assert.Equal(t, id, r.idOf(web.Entity), "one ID for the flicker")
	assert.Len(t, r.m.Export(), 1)
	assert.Equal(t, Settling, r.of(web.Entity).State)
}

// Once it stays, it is announced under that same ID, once.
func TestRevivedIncidentIsAnnouncedOnceUnderItsID(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	id := r.idOf(web.Entity)
	r.clear(at(10*time.Second), web)
	r.tick(at(10 * time.Second))
	back := at(20 * time.Second)
	r.raise(back, web)

	ds := r.tick(back.Add(DefaultSettle))

	wantAction(t, ds, Announce, ReasonSettled)
	assert.Equal(t, id, ds[0].Incident.ID)
	assert.NotEmpty(t, r.of(web.Entity).History, "the blip is remembered")
}

// An incident that was announced is never revived: people heard of it, so
// the usual reopen or recurrence rules apply.
func TestAnnouncedIncidentIsNotRevived(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	now := at(DefaultSettle + 10*time.Second)
	r.clear(now, web)
	r.tick(now)
	now = now.Add(DefaultHold + time.Second)
	require.Equal(t, Resolve, r.tick(now)[0].Action)

	now = now.Add(RepageWindow + time.Hour)
	r.raise(now, web)

	assert.NotEqual(t, id, r.idOf(web.Entity), "a recurrence has a new ID")
}
