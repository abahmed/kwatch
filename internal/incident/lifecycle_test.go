package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

func TestManagerSettlesThenAnnouncesOnce(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))

	ds, next := r.m.Tick(at(10 * time.Second))
	wantNone(t, ds)
	if next != 65*time.Second {
		t.Fatalf("next wake = %v, want 65s", next)
	}
	if r.only().State != Settling {
		t.Fatal("incident should still be settling")
	}

	ds = r.tick(at(DefaultSettle))
	wantAction(t, ds, Announce, "settled")
	got := ds[0].Incident
	if got.State != Open || got.Revision != 1 || got.Tier != Notify {
		t.Fatalf("unexpected announced incident: %+v", got)
	}
	wantNone(t, r.tick(at(2*DefaultSettle)))
}

func TestManagerPageTierSettlesFaster(t *testing.T) {
	r := newRig(t, Config{})
	node := entity("node", "n1")
	r.raise(at(0), sig(node, "NodeNotReady", detection.Critical))

	wantNone(t, r.tick(at(DefaultPageSettle-time.Second)))
	ds := r.tick(at(DefaultPageSettle))
	wantAction(t, ds, Announce, "settled")
	if ds[0].Incident.Tier != Page {
		t.Fatalf("tier = %v, want Page", ds[0].Incident.Tier)
	}
}

func TestManagerSilentTierNeverAnnounces(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	r.m.mu.Lock()
	for _, p := range r.m.incidents {
		p.Tier = Silent
	}
	r.m.mu.Unlock()

	wantNone(t, r.tick(at(time.Hour)))
	if r.only().State != Settling {
		t.Fatal("silent incident should stay settling")
	}
}

func TestManagerRecoveryBeforeAnnouncementStaysSilent(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	r.clear(at(10*time.Second), web)

	wantNone(t, r.tick(at(DefaultSettle)))
	if got := r.only(); got.State != Resolved || got.Revision != 0 {
		t.Fatalf("want silently resolved, got %+v", got)
	}
}

func TestManagerResolvesAfterHold(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")

	gone := at(100 * time.Second)
	r.clear(gone, web)
	r.tick(gone)
	if r.only().State != Recovering {
		t.Fatal("incident should be recovering")
	}
	wantNone(t, r.tick(gone.Add(time.Second)))
	wantNone(t, r.tick(gone.Add(DefaultHold-time.Second)))

	ds := r.tick(gone.Add(DefaultHold))
	wantAction(t, ds, Resolve, "healthy for 3m0s")
	got := ds[0].Incident
	if got.State != Resolved || got.Revision != 2 {
		t.Fatalf("unexpected resolved incident: %+v", got)
	}
	last := got.Timeline[len(got.Timeline)-1].Text
	if !strings.HasPrefix(last, "resolved: ") {
		t.Fatalf("timeline should end with resolve, got %q", last)
	}
	wantNone(t, r.tick(gone.Add(2*DefaultHold)))
}

func TestManagerRecurrenceInsideHoldIsSilent(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	r.tick(at(DefaultSettle))

	r.clear(at(100*time.Second), web)
	r.tick(at(100 * time.Second))
	r.raise(at(110*time.Second), web)

	wantNone(t, r.tick(at(110*time.Second)))
	got := r.only()
	if got.State != Open || len(got.Cycles) != 1 {
		t.Fatalf("want open with one cycle, got %+v", got)
	}
}

func TestManagerFlappingIncidentGoesQuietThenResolves(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	r.tick(at(DefaultSettle))

	now := at(100 * time.Second)
	var ds []Decision
	for i := 0; i < DefaultFlapCycles; i++ {
		r.clear(now, web)
		wantNone(t, r.tick(now))
		now = now.Add(10 * time.Second)
		r.raise(now, web)
		ds = r.tick(now)
	}
	wantAction(t, ds, Update, "flapping")
	if r.only().State != Flapping {
		t.Fatal("incident should be flapping")
	}
	wantNone(t, r.tick(now.Add(time.Minute)))

	r.clear(now.Add(time.Minute), web)
	quiet := now.Add(time.Minute)
	_, next := r.m.Tick(quiet)
	if next != DefaultMaxHold {
		t.Fatalf("next wake = %v, want %v", next, DefaultMaxHold)
	}
	wantNone(t, r.tick(quiet.Add(DefaultMaxHold-time.Second)))
	ds = r.tick(quiet.Add(DefaultMaxHold))
	wantAction(t, ds, Resolve, "stable for 30m0s")
}

func TestManagerFlappingResetsQuietWhenFailingAgain(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	r.tick(at(DefaultSettle))
	r.m.mu.Lock()
	p := r.m.lookup(web.Entity)
	p.State = Flapping
	r.m.mu.Unlock()

	r.clear(at(time.Minute), web)
	r.tick(at(time.Minute))
	r.raise(at(2*time.Minute), web)
	wantNone(t, r.tick(at(2*time.Minute)))
	if !r.only().RecoveringSince.IsZero() {
		t.Fatal("failing again should reset the quiet period")
	}
}

func TestManagerForgetsResolvedIncidentsAfterRemember(t *testing.T) {
	r := newRig(t, Config{Remember: time.Hour})
	web := podSig("web")
	r.raise(at(0), web)
	r.clear(at(time.Second), web)
	r.tick(at(DefaultSettle))
	if len(r.m.Export()) != 1 {
		t.Fatal("resolved incident should be remembered")
	}
	r.tick(at(DefaultSettle + time.Hour + time.Second))
	if n := len(r.m.Export()); n != 0 {
		t.Fatalf("incident should be forgotten, %d left", n)
	}
}

func TestManagerRecurrenceOpensLinkedIncident(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	r.clear(at(time.Second), web)
	r.tick(at(DefaultSettle))
	first := r.idOf(web.Entity)

	again := at(2 * time.Hour)
	r.raise(again, web)
	got := r.of(web.Entity)
	if got.ID == first || got.Previous != first {
		t.Fatalf("recurrence should be a new incident linked to %s: %+v",
			first, got)
	}
	if got.State != Settling || got.Revision != 0 || got.Digest != "" {
		t.Fatalf("recurrence should restart settling: %+v", got)
	}
	if len(got.Occurrences) != 2 {
		t.Fatalf("occurrences = %d, want 2", len(got.Occurrences))
	}
	found := false
	for _, e := range got.Timeline {
		found = found ||
			e.Text == "happened again (2nd time this week, last time "+
				first+")"
	}
	if !found {
		t.Fatalf("timeline lacks recurrence note: %+v", got.Timeline)
	}
	wantAction(t, r.tick(again.Add(DefaultSettle)), Announce, "settled")
}
