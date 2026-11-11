package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An incident whose announcement the scope dropped is announced when
// the scope comes back, without waiting for its weekly reminder.
func TestUnheardIncidentIsAnnouncedWhenItsScopeReturns(t *testing.T) {
	r, start := announcedRig(t)
	id := r.only().ID
	r.m.RecordScope(id, false)
	wantNone(t, r.tick(start.Add(time.Hour)))
	require.Len(t, r.m.Unheard(), 1)

	back := start.Add(2 * time.Hour)
	d, ok := r.m.AnnounceUnheard(id, back)

	require.True(t, ok)
	assert.Equal(t, Announce, d.Action)
	assert.Equal(t, ScopeIn, d.Incident.Scope)
	assert.Equal(t, back, d.Incident.Announced)
	assert.Empty(t, r.m.Unheard(), "told once")
	_, again := r.m.AnnounceUnheard(id, back.Add(time.Minute))
	assert.False(t, again)
	// Reminders count from the announcement people got.
	wantNone(t, r.tick(start.Add(RemindEvery)))
	wantAction(t, r.tick(back.Add(RemindEvery)), Update, ReasonReminder)
}

// Only a failing, announced, unheard incident is asked about again.
func TestUnheardLeavesOutWhatHasNothingToAnnounce(t *testing.T) {
	r, start := announcedRig(t)
	assert.Empty(t, r.m.Unheard(), "delivered")

	r.m.RecordScope(r.only().ID, false)
	r.clear(start.Add(time.Minute), podSig("web"))
	r.tick(start.Add(time.Minute))

	assert.Empty(t, r.m.Unheard(), "recovering: nothing fails")
	_, ok := r.m.AnnounceUnheard(r.only().ID, start.Add(2*time.Minute))
	assert.False(t, ok)
}
