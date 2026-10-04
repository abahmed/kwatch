package incident

import (
	"testing"
	"time"
)

func TestManagerTickOpenToRecoveringReturnsHold(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")

	gone := at(100 * time.Second)
	r.clear(gone, web)
	ds, next := r.m.Tick(gone)
	wantNone(t, ds)
	if r.only().State != Recovering {
		t.Fatal("incident should be recovering")
	}
	if next != DefaultHold {
		t.Fatalf("next wake = %v, want hold %v", next, DefaultHold)
	}
}

func TestManagerTickSettlingReturnsSettleDeadline(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	_, next := r.m.Tick(at(5 * time.Second))
	if want := DefaultSettle - 5*time.Second; next != want {
		t.Fatalf("next wake = %v, want %v", next, want)
	}
}

func TestManagerTickWithoutTimersReturnsZero(t *testing.T) {
	r := newRig(t, Config{Remember: time.Hour})
	if _, next := r.m.Tick(at(0)); next != 0 {
		t.Fatalf("empty manager next wake = %v, want 0", next)
	}
}

// An open incident's only timer is its weekly reminder.
func TestManagerTickOpenIncidentWakesForItsReminder(t *testing.T) {
	r := newRig(t, Config{Remember: time.Hour})
	r.raise(at(0), podSig("web"))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	if _, next := r.m.Tick(at(DefaultSettle)); next != RemindEvery {
		t.Fatalf("open incident next wake = %v, want %v", next, RemindEvery)
	}
}

func TestManagerTickEarliestDeadlineWins(t *testing.T) {
	r := newRig(t, Config{})
	web, db := podSig("web"), podSig("db")
	r.raise(at(0), web)
	r.raise(at(20*time.Second), db)
	_, next := r.m.Tick(at(30 * time.Second))
	if want := DefaultSettle - 30*time.Second; next != want {
		t.Fatalf("next wake = %v, want earliest %v", next, want)
	}
}

func TestManagerTickResolvedReportsForgetDeadline(t *testing.T) {
	r := newRig(t, Config{Remember: time.Hour})
	web := podSig("web")
	r.raise(at(0), web)
	r.clear(at(time.Second), web)
	_, next := r.m.Tick(at(2 * time.Second))
	if want := time.Hour + time.Nanosecond; next != want {
		t.Fatalf("next wake = %v, want %v", next, want)
	}
	forget := at(2 * time.Second).Add(next)
	if _, n := r.m.Tick(forget); n != 0 || len(r.m.Export()) != 0 {
		t.Fatalf("resolved incident kept after remember window")
	}
}

func TestManagerTickRestoreGraceReportsGraceEnd(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.m.Restore(r.m.Export(), at(DefaultSettle+time.Minute))
	_, next := r.m.Tick(at(DefaultSettle))
	if next != time.Minute {
		t.Fatalf("next wake = %v, want grace end 1m", next)
	}
}
