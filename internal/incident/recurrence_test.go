package incident

import (
	"testing"
	"time"
)

// runCycles fails for fail and recovers for healthy, ticking every 10s,
// and returns every decision.
func runCycles(
	r *rig, fail, healthy, until time.Duration,
) []Decision {
	web := podSig("web")
	var out []Decision
	period := fail + healthy
	failing := false
	for now := time.Duration(0); now <= until; now += 10 * time.Second {
		want := now%period < fail
		switch {
		case want && !failing:
			r.raise(at(now), web)
		case !want && failing:
			r.clear(at(now), web)
		}
		failing = want
		out = append(out, r.tick(at(now))...)
	}
	return out
}

func TestManagerRecurrenceAfterResolveCountsAsFlapCycle(t *testing.T) {
	r := newRig(t, Config{})

	ds := runCycles(r, 2*time.Minute, 4*time.Minute, 30*time.Minute)

	counts := map[Action]int{}
	flapping := false
	for _, d := range ds {
		counts[d.Action]++
		flapping = flapping || d.Incident.State == Flapping
	}
	if !flapping {
		t.Fatalf("2m fail / 4m healthy must become flapping: %+v", counts)
	}
	if counts[Announce] > 2 || counts[Resolve] > 1 {
		t.Fatalf("hold should double on recurrence: %+v", counts)
	}
}

func TestManagerRecurrenceOutsideFlapWindowIsNotACycle(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	wantAction(t, r.tick(at(2*time.Minute+DefaultHold)), Resolve,
		"healthy for 3m0s")

	r.raise(at(2*time.Hour), web)

	if got := r.of(web.Entity); len(got.Cycles) != 0 {
		t.Fatalf("cycles = %v, want none", got.Cycles)
	}
}

func TestManagerRecurrenceStartsItsOwnConversation(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	first := r.idOf(web.Entity)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	ds := r.tick(at(2*time.Minute + DefaultHold))
	wantAction(t, ds, Resolve, "healthy for 3m0s")

	again := at(time.Hour)
	r.raise(again, web)
	ds = r.tick(again.Add(DefaultSettle))
	wantAction(t, ds, Announce, "settled")
	got := ds[0].Incident
	if got.ID == first || got.Previous != first || got.Revision != 1 {
		t.Fatalf("recurrence should open a linked incident: %+v", got)
	}
	if len(got.Occurrences) != 2 {
		t.Fatalf("occurrences = %v, want both", got.Occurrences)
	}
}
