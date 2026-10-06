package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// returnAfter raises the failure again 5 minutes after the resolve at
// resolved and returns the incident that holds it.
func returnAfter(t *testing.T, r *rig, resolved time.Duration) Incident {
	t.Helper()
	raiseAt := resolved + 5*time.Minute
	r.raise(at(raiseAt), nodeDown())
	tickEvery(r, raiseAt, raiseAt+DefaultSettle+time.Minute)
	return r.only()
}

// An announcement that was held and resolved before anyone was told is
// not a thing people heard of: its return is a new incident, not "failing
// again".
func TestAnnouncementResolvedWhileHeldIsNotReopened(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	r.m.HoldAnnouncement(id)
	resolved := resolvePage(t, r, 5*time.Minute)
	r.m.DropAnnouncement(id)

	again := returnAfter(t, r, resolved)
	assert.NotEqual(t, id, again.ID, "nobody was told, so no reopen")
	for _, o := range again.History {
		assert.False(t, o.Heard, "the earlier occurrence was never heard")
	}
}

// An announcement dropped as out of scope was never delivered either.
func TestOutOfScopeAnnouncementIsNotReopened(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	r.m.RecordScope(id, false)
	resolved := resolvePage(t, r, 5*time.Minute)

	again := returnAfter(t, r, resolved)
	assert.NotEqual(t, id, again.ID)
}

// The told flag survives a restart; a record written before it existed
// restores as told.
func TestToldSurvivesRestore(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	records := r.m.Export()
	require.Len(t, records, 1)
	assert.False(t, records[0].Unheard)

	r.m.RecordScope(id, false)
	records = r.m.Export()
	assert.True(t, records[0].Unheard)

	restored := NewManager(Config{IDNonce: testNonce}, nil)
	restored.Restore(records, at(0))
	assert.False(t, restored.Incidents()[0].CanReopen())
}

// A held record saved before ToldKnown existed was never told, so it
// restores as not told; a held record that carries the field keeps what
// it says, and an unheld old record still restores as told.
func TestOldHeldRecordRestoresAsNotTold(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	r.m.HoldAnnouncement(id)
	rec := r.m.Export()[0]
	require.True(t, rec.Held)
	require.True(t, rec.ToldKnown)

	assert.True(t, rec.heard() == !rec.Unheard)
	rec.ToldKnown, rec.Unheard = false, false
	assert.False(t, rec.heard(), "old held record: not told")

	rec.Held = false
	assert.True(t, rec.heard(), "old unheld record: told")
}
