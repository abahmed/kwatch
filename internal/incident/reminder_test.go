package incident

import (
	"testing"
	"time"
)

// announcedRig opens a rig with one announced incident and returns the
// time of the announcement.
func announcedRig(t *testing.T) (*rig, time.Time) {
	t.Helper()
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	return r, r.only().Announced
}

func TestManagerRemindsOnceAWeekWhileOpen(t *testing.T) {
	r, start := announcedRig(t)

	wantNone(t, r.tick(start.Add(RemindEvery-time.Second)))
	first := start.Add(RemindEvery)
	ds := r.tick(first)
	wantAction(t, ds, Update, ReasonReminder)
	if ds[0].Incident.Reminded != first {
		t.Fatalf("Reminded = %v, want %v", ds[0].Incident.Reminded, first)
	}

	wantNone(t, r.tick(first.Add(RemindEvery-time.Second)))
	wantAction(t, r.tick(first.Add(RemindEvery)), Update, ReasonReminder)
}

func TestManagerReminderWakesTheTickAtItsDeadline(t *testing.T) {
	r, start := announcedRig(t)

	_, next := r.m.Tick(start.Add(time.Hour))

	if want := RemindEvery - time.Hour; next != want {
		t.Fatalf("next wake = %v, want %v", next, want)
	}
}

func TestManagerReminderSurvivesRestore(t *testing.T) {
	src, start := announcedRig(t)
	reminded := start.Add(RemindEvery)
	wantAction(t, src.tick(reminded), Update, ReasonReminder)

	dst := newRig(t, Config{})
	dst.m.Restore(src.m.Export(), time.Time{})

	if got := dst.only().Reminded; !got.Equal(reminded) {
		t.Fatalf("restored Reminded = %v, want %v", got, reminded)
	}
}
