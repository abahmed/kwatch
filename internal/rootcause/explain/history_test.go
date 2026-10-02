package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestHistoryChangesReadsTheEntitysChanges(t *testing.T) {
	f := newFixture(t)
	secret := inventory.CoreID(kube.KindSecret, "shop", "db-creds")
	other := inventory.CoreID(kube.KindSecret, "shop", "other")
	f.add(secret, other)
	for _, id := range []inventory.EntityID{secret, other} {
		f.apply(inventory.Observation{Kind: inventory.Changed, Entity: id,
			Change: inventory.Change{Entity: id,
				At: t0.Add(time.Minute)}})
	}
	h := HistoryChanges{History: f.model}
	got := h.Changes(secret, t0, t0.Add(10*time.Minute))
	if len(got) != 1 || got[0].Entity != secret {
		t.Fatalf("changes = %+v, want the secret's own", got)
	}
	outcome, ok := h.Outcome(got[0], t0.Add(2*time.Minute))
	if !ok || outcome != inventory.OutcomePending {
		t.Fatalf("outcome = %q (ok %v), want pending", outcome, ok)
	}
	unknown := inventory.Change{Entity: secret, At: t0.Add(time.Hour)}
	if _, ok := h.Outcome(unknown, t0.Add(2*time.Hour)); ok {
		t.Fatal("a change in no set has no outcome")
	}
}

// fakeOutcomes judges every change alike.
type fakeOutcomes inventory.Outcome

func (o fakeOutcomes) Outcome(
	inventory.Change, time.Time,
) (inventory.Outcome, bool) {
	return inventory.Outcome(o), o != ""
}

func TestScoreOutcomeReadsTheReleaseOutcome(t *testing.T) {
	cases := map[inventory.Outcome]float64{
		inventory.OutcomeReverted: OutcomeRevertedWeight,
		inventory.OutcomeDegraded: OutcomeDegradedWeight,
		inventory.OutcomeHealthy:  0,
		inventory.OutcomePending:  0,
	}
	for outcome, want := range cases {
		t.Run(string(outcome), func(t *testing.T) {
			f := newFixture(t)
			usesCase(f, kube.KindSecret, true)
			s := f.snapshot()
			s.Outcomes = fakeOutcomes(outcome)
			secret := inventory.CoreID(kube.KindSecret, "shop", "db-creds")
			requireWeight(t, scoreSnapshot(t, s, secret, scoreOutcome), want)
		})
	}
}

func TestWorkloadBaselinesFlagsAnUnusualWait(t *testing.T) {
	f := newFixture(t)
	pod := f.workload("shop", "api", 1)[0]
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
		Attributes: map[string]inventory.Value{
			kube.AttrCreated: inventory.Time(t0)}})
	b := WorkloadBaselines{History: f.model, Now: t0.Add(10 * time.Minute)}
	if _, ok := b.Deviation(deployment); ok {
		t.Fatal("a workload without history is not judged")
	}
	for i := range inventory.MinBaselineSamples {
		f.model.Baselines().Add(deployment, inventory.MetricPendingSeconds,
			5, t0.Add(time.Duration(i)*time.Minute))
	}
	if d, ok := b.Deviation(deployment); !ok || d != 1 {
		t.Fatalf("deviation = %v (ok %v), want a 10m wait to be unusual",
			d, ok)
	}
	b.Now = t0.Add(6 * time.Second)
	if _, ok := b.Deviation(deployment); ok {
		t.Fatal("a usual wait is no evidence")
	}
}

func TestInvisibleKindIsNotedNotBlamed(t *testing.T) {
	f := newFixture(t)
	effect := usesCase(f, kube.KindSecret, false)
	s := f.snapshot()
	s.Verifiable = func(kind inventory.Kind) bool {
		return kind != kube.KindSecret
	}
	e := Explain(s)
	if c, ok := e.CauseOf(effect); ok && c.Root.Kind == kube.KindSecret {
		t.Fatalf("an invisible secret was blamed: %+v", c)
	}
	if len(e.Areas) == 0 ||
		len(e.Areas[0].Unverified) != 1 ||
		e.Areas[0].Unverified[0] != "secrets in shop" {
		t.Fatalf("areas = %+v, want the gap noted", e.Areas)
	}
}
