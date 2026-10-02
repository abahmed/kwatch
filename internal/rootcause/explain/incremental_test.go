package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeOutage fails n1 and the pods on it.
func nodeOutage(f *fixture) (inventory.EntityID, []inventory.EntityID) {
	node, pods := nodeWithPods(f, 2, "api", "web")
	f.fail(node, "NotReady", failingH, 1, "")
	for _, pod := range pods {
		f.fail(pod, "NotReady", degradedH, 2, "")
	}
	return node, pods
}

func TestSolverSkipsASteadySnapshot(t *testing.T) {
	f := newFixture(t)
	nodeOutage(f)
	sv := NewSolver()
	if got := sv.Solve(f.snapshot(), nil); len(got) != 1 {
		t.Fatalf("first solve returned %d areas, want 1", len(got))
	}
	if got := sv.Solve(f.snapshot(), nil); len(got) != 0 {
		t.Fatalf("steady solve returned %d areas, want 0", len(got))
	}
}

func TestSolverSolvesAgainWhenAnInputMoves(t *testing.T) {
	f := newFixture(t)
	node, _ := nodeOutage(f)
	sv := NewSolver()
	sv.Solve(f.snapshot(), nil)
	got := sv.Solve(f.snapshot(), []inventory.EntityID{node})
	if len(got) != 1 || len(got[0].Causes) != 1 ||
		got[0].Causes[0].Root != node {
		t.Fatalf("areas = %+v, want the node area again", got)
	}
}

func TestSolverDropsAHealedFailure(t *testing.T) {
	f := newFixture(t)
	_, pods := nodeOutage(f)
	sv := NewSolver()
	sv.Solve(f.snapshot(), nil)
	delete(f.findings, pods[0])
	got := sv.Solve(f.snapshot(), nil)
	if len(got) != 1 || containsID(got[0].Failures, pods[0]) {
		t.Fatalf("areas = %+v, want one area without the healed pod",
			got)
	}
}

func TestSolverMergesANewFailureWithItsSharedCause(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeOutage(f)
	late := pods[len(pods)-1]
	early := f.findings[late]
	delete(f.findings, late)
	sv := NewSolver()
	sv.Solve(f.snapshot(), nil)

	f.findings[late] = early
	got := sv.Solve(f.snapshot(), []inventory.EntityID{late})
	if len(got) != 1 || !containsID(got[0].Failures, pods[0]) ||
		!containsID(got[0].Failures, late) {
		t.Fatalf("areas = %+v, want one merged area", got)
	}
	if c, ok := (Explanation{Areas: sv.Areas()}).CauseOf(late); !ok ||
		c.Root != node {
		t.Fatalf("the late pod is not explained by the node: %+v", c)
	}
	if n := len(sv.Areas()); n != 1 {
		t.Fatalf("cached areas = %d, want 1", n)
	}
}

// TestSolverSolvesAgainWhenAnUpstreamCauseFailsLater: the autoscaler
// starts failing after the workload it limits was cached as
// unexplained, and the cached area must take it as its cause.
func TestSolverSolvesAgainWhenAnUpstreamCauseFailsLater(t *testing.T) {
	f := newFixture(t)
	ceilingCase(f)
	hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.fail(deployment, "Unavailable", degradedH, 3, "")
	scaling := f.findings[hpa]
	delete(f.findings, hpa)
	sv := NewSolver()
	sv.Solve(f.snapshot(), nil)

	f.findings[hpa] = scaling
	sv.Solve(f.snapshot(), []inventory.EntityID{hpa})
	c, ok := (Explanation{Areas: sv.Areas()}).CauseOf(deployment)
	if !ok || c.Root != hpa {
		t.Fatalf("deployment cause = %+v, %v; want the autoscaler", c, ok)
	}
	if n := len(sv.Areas()); n != 1 {
		t.Fatalf("cached areas = %d, want 1", n)
	}
}
