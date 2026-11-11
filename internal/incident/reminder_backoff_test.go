package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func unusedPod(name string) detection.Finding {
	return sig(entity(kube.KindPod, name), reasons.ServiceUnused,
		detection.Warning)
}

// digestRig announces one digest-tier incident and returns its start.
func digestRig(t *testing.T) (*rig, time.Time) {
	t.Helper()
	r := newRig(t, Config{})
	r.raise(at(0), unusedPod("web"))
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 1)
	require.Equal(t, Digest, ds[0].Incident.Tier)
	return r, r.only().Announced
}

// A quiet digest-tier incident is reminded after a day, then 3 days,
// then a week, and the week repeats.
func TestDigestReminderBacksOff(t *testing.T) {
	r, start := digestRig(t)
	last := start
	for _, gap := range []time.Duration{format.Day, 3 * format.Day,
		format.Week, format.Week} {
		wantNone(t, r.tick(last.Add(gap-time.Second)))
		wantAction(t, r.tick(last.Add(gap)), Update, ReasonReminder)
		last = last.Add(gap)
	}
}

// News resets the gaps: after a message that is not a reminder the
// next reminder comes after a day again.
func TestDigestReminderBackoffResetsOnNews(t *testing.T) {
	r, start := digestRig(t)
	wantAction(t, r.tick(start.Add(format.Day)), Update, ReasonReminder)
	assert.Equal(t, 1, r.only().Reminders)
	r.m.incidents[r.only().ID].resetBackoff(ReasonMaterialChange)
	assert.Zero(t, r.only().Reminders)
	assert.Equal(t, format.Day, chronicBackoff(r.only().Reminders))
}

// The count survives a restart; a record without it takes the first gap.
func TestReminderCountIsPersisted(t *testing.T) {
	r, start := digestRig(t)
	wantAction(t, r.tick(start.Add(format.Day)), Update, ReasonReminder)
	records := r.m.Export()
	require.Len(t, records, 1)
	assert.Equal(t, 1, records[0].Reminders)

	dst := newRig(t, Config{})
	dst.m.Restore(records, time.Time{})
	assert.Equal(t, 1, dst.only().Reminders)
	got := dst.only()
	assert.Equal(t, 3*format.Day, reminderEvery(&got))

	records[0].Reminders = 0
	old := newRig(t, Config{})
	old.m.Restore(records, time.Time{})
	gotOld := old.only()
	assert.Equal(t, format.Day, reminderEvery(&gotOld))
}

// Notify-tier incidents keep their weekly reminder.
func TestNotifyReminderCadenceIsUnchanged(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	r.tick(at(DefaultSettle))
	start := r.only().Announced
	wantNone(t, r.tick(start.Add(RemindEvery-time.Second)))
	wantAction(t, r.tick(start.Add(RemindEvery)), Update, ReasonReminder)
	wantNone(t, r.tick(start.Add(2*RemindEvery-time.Second)))
	wantAction(t, r.tick(start.Add(2*RemindEvery)), Update, ReasonReminder)
}
