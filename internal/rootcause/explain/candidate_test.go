package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestCandidateRejectNotesAreStable: when a row drops several effects
// of one candidate, the note kept names the same effect on every run.
func TestCandidateRejectNotesAreStable(t *testing.T) {
	f := newFixture(t)
	effects := callStorm(f, 1, refusedDB)
	endpoint := inventory.CoreID(KindExternalEndpoint, "",
		"db.example.com:5432")
	want := effects[0]
	for _, effect := range effects[1:] {
		if effect.String() < want.String() {
			want = effect
		}
	}
	for i := 0; i < 50; i++ {
		v := newView(f.snapshot())
		cs := v.candidates(failures(f.findings))
		note, ok := cs.rejected[endpoint]
		if !ok || note.effect != want {
			t.Fatalf("run %d: note = %+v (%v), want one for %s", i, note,
				ok, want)
		}
	}
}

// TestCandidateSummariesFollowPrunedEffects: a row that needs two
// failures and finds one explains neither the pod nor the summary
// finding of the pod's Deployment.
func TestCandidateSummariesFollowPrunedEffects(t *testing.T) {
	f := newFixture(t)
	f.fail(kube.Scheduler, "Unavailable.Scheduler", failingH, 1, "")
	pod := f.workload("shop", "api", 1)[0]
	f.fail(pod, "Pending", degradedH, 2, "")
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")
	f.fail(deployment, detection.ModeUnavailable, degradedH, 2, "")
	v := newView(f.snapshot())
	cs := v.candidates(failures(f.findings))
	if c, ok := cs.byID[kube.Scheduler]; ok {
		if _, covered := c.covers[deployment]; covered {
			t.Fatalf("scheduler covers %s through a pruned pod", deployment)
		}
	}
}

// TestViewGateIsIdempotent: gating one hop twice lists the gate once.
func TestViewGateIsIdempotent(t *testing.T) {
	f := newFixture(t)
	pod := f.workload("shop", "api", 1)[0]
	f.fail(containerOf(pod), "CrashLoop", failingH, 2, "")
	v := newView(f.snapshot())
	v.unitGate(pod)
	v.unitGate(pod)
	if got := v.gates[pod]; len(got) != 1 {
		t.Fatalf("gates[pod] = %v, want the container once", got)
	}
}
