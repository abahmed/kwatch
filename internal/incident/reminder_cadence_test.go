package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/format"
)

// rolledUpRig announces one notify-tier incident that a listing carried.
func rolledUpRig(t *testing.T) (*rig, string, time.Time) {
	t.Helper()
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	id := r.only().ID
	r.m.RecordRolledUp(id)
	return r, id, r.only().Announced
}

// An incident a listing only named backs off while it has no message of
// its own: its first reminder comes after a day. That reminder is its
// own message once it reaches chat (delivery notes it, NoteSaid), so
// from then on it is reminded weekly like any other, not after 3 days.
func TestListedIncidentBacksOffOnlyUntilItSpoke(t *testing.T) {
	r, id, start := rolledUpRig(t)
	first := start.Add(ChronicRemindEvery)
	wantNone(t, r.tick(first.Add(-time.Second)))
	wantAction(t, r.tick(first), Update, ReasonReminder)
	r.m.NoteSaid(id, r.only().Digest)

	wantNone(t, r.tick(first.Add(3*format.Day)))
	wantNone(t, r.tick(first.Add(RemindEvery-time.Second)))
	wantAction(t, r.tick(first.Add(RemindEvery)), Update, ReasonReminder)
}

// An introduction or a lowered tier tells nothing new about the
// problem: neither starts the reminder gaps over.
func TestIntroductionKeepsTheReminderBackoff(t *testing.T) {
	r, start := digestRig(t)
	wantAction(t, r.tick(start.Add(format.Day)), Update, ReasonReminder)
	id := r.only().ID
	r.m.RecordRolledUp(id)
	_, ok := r.m.Introduce(id)
	require.True(t, ok)
	assert.Equal(t, 1, r.only().Reminders, "the introduction reset it")

	p := r.m.incidents[id]
	p.resetBackoff(ReasonTierLowered)
	assert.Equal(t, 1, p.Reminders, "a lowered tier reset it")
	p.resetBackoff(ReasonMaterialChange)
	assert.Zero(t, p.Reminders, "news starts the gaps over")
}

// A problem with no news for UnchangedFor is reminded at most once per
// UnchangedFor: weekly at first, then monthly.
func TestUnchangedProblemIsRemindedMonthly(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	start := r.only().Announced
	last := start
	for range 4 {
		last = last.Add(RemindEvery)
		wantAction(t, r.tick(last), Update, ReasonReminder)
	}
	for _, d := range []time.Duration{RemindEvery, 2 * RemindEvery,
		UnchangedFor - time.Second} {
		wantNone(t, r.tick(last.Add(d)))
	}
	wantAction(t, r.tick(last.Add(UnchangedFor)), Update, ReasonReminder)
}

// A reminder that fell due while kwatch was down is not sent in the
// first minutes after the start, inside the restore grace: it waits
// for the end of the grace.
func TestNoReminderInsideTheRestoreGrace(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	start := r.only().Announced
	records := r.m.Export()

	restart := start.Add(RemindEvery + format.Day)
	grace := restart.Add(10 * time.Minute)
	again := newRig(t, Config{})
	again.m.Restore(records, grace)
	again.raise(restart, web)
	for _, d := range []time.Duration{0, time.Minute, 9 * time.Minute} {
		wantNone(t, again.tick(restart.Add(d)))
	}
	wantAction(t, again.tick(grace), Update, ReasonReminder)
}
