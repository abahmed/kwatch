package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// easedPage pages for a crashing web, then lets the previous version
// serve again and re-raises the same failure, so the incident is
// re-evaluated (easePage) with nothing else new.
func easedPage(t *testing.T) *impactRig {
	t.Helper()
	r := newImpactRig(t, "LoadBalancer", false, 1, 0, -1, 0)
	r.crash()
	for m := 1; m <= 5; m++ {
		r.tick(at(time.Duration(m) * time.Minute))
	}
	if got := r.only(); got.Tier != Page {
		t.Fatalf("tier = %v, want page while nothing serves", got.Tier)
	}
	r.rolloutStuck(1)
	r.raise(at(6*time.Minute),
		sig(r.pod, reasons.CrashLoopBackOff, detection.Critical))
	return r
}

// decisionsFrom ticks every minute from minute from to 40 and returns
// the decisions made.
func decisionsFrom(r *impactRig, from int) []Decision {
	var out []Decision
	for m := from; m <= 40; m++ {
		out = append(out, r.tick(at(time.Duration(m)*time.Minute))...)
	}
	return out
}

// A page that eases because the previous version serves again owes the
// pagers exactly one "tier lowered" update, at the lower tier, and no
// later material change about the tier.
func TestEasedPageSendsOneLoweringUpdate(t *testing.T) {
	ds := decisionsFrom(easedPage(t), 7)
	if len(ds) != 1 {
		t.Fatalf("decisions = %d, want one: %+v", len(ds), ds)
	}
	d := ds[0]
	if d.Action != Update || d.Reason != ReasonTierLowered ||
		d.Incident.Tier != Notify {
		t.Fatalf("got %v %q at %v; want update %q at notify",
			d.Action, d.Reason, d.Incident.Tier, ReasonTierLowered)
	}
}

// When other news is pending as the page eases, one material update
// tells everyone (the pagers take its lower tier), not a lowering the
// chat would miss, and MaterialGap does not hold it.
func TestEasedPageWithOtherNewsIsOneMaterialUpdate(t *testing.T) {
	r := easedPage(t)
	r.m.mu.Lock()
	for _, p := range r.m.incidents {
		p.Digest = "news-pending"
	}
	r.m.mu.Unlock()
	ds := decisionsFrom(r, 7)
	if len(ds) != 1 || ds[0].Reason != ReasonMaterialChange ||
		ds[0].Incident.Tier != Notify {
		t.Fatalf("decisions: %+v; want one material change at notify",
			ds)
	}
}
