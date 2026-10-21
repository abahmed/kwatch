package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// causeIn names the cause of a stuck object in the cached areas.
func causeIn(sv *Solver, id inventory.EntityID) Cause {
	for _, area := range sv.Areas() {
		for _, c := range area.Causes {
			if containsID(c.Covers, id) {
				return c
			}
		}
	}
	return Cause{}
}

// A controller that shows up after its leftovers were solved is a new
// handler: the cached area was solved without it, so it must be solved
// again when the controller appears.
func TestSolverSolvesAgainWhenTheHandlerAppearsLater(t *testing.T) {
	f := newFixture(t)
	stuck := f.stuck("ops", 3, widgetFinalizer)
	sv := NewSolver()
	sv.Solve(f.snapshot(), nil)
	if got := causeIn(sv, stuck[0]); got.Root.Kind != KindFinalizer {
		t.Fatalf("before: cause = %v, want the finalizer", got.Root)
	}

	ctl := f.controller(kube.KindDeployment, "tools", "widget-ctl", 0, 0,
		"widgets.example.com/controller-namespace=ops")
	sv.Solve(f.snapshot(), []inventory.EntityID{ctl})

	if got := causeIn(sv, stuck[0]); got.Root != ctl {
		t.Fatalf("after: cause = %v, want %v", got.Root, ctl)
	}
}

// The same holds for the evidence a handler is known by: a role granted
// to its account later makes it the handler.
func TestSolverSolvesAgainWhenTheHandlersRoleAppearsLater(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "arc-system", "gha-controller",
		0, 0, "app=gha")
	account := f.accountOf(ctl)
	stuck := f.runnerSet()
	sv := NewSolver()
	sv.Solve(f.snapshot(), nil)
	if got := causeIn(sv, stuck[0]); got.Root.Kind != KindFinalizer {
		t.Fatalf("before: cause = %v, want the finalizer", got.Root)
	}

	f.grant(account, arcGroup)
	binding := inventory.CoreID(kube.KindClusterRoleBinding, "", account.Name)
	sv.Solve(f.snapshot(), []inventory.EntityID{binding})

	if got := causeIn(sv, stuck[0]); got.Root != ctl {
		t.Fatalf("after: cause = %v, want %v", got.Root, ctl)
	}
}

// An unrelated kind moving does not solve a finalizer area again.
func TestSolverKeepsAFinalizerAreaWhenAnUnrelatedEntityMoves(t *testing.T) {
	f := newFixture(t)
	f.stuck("ops", 3, widgetFinalizer)
	sv := NewSolver()
	sv.Solve(f.snapshot(), nil)
	secret := inventory.CoreID(kube.KindSecret, "ops", "unrelated")
	f.add(secret)

	if got := sv.Solve(f.snapshot(), []inventory.EntityID{secret}); len(got) != 0 {
		t.Fatalf("solved %d areas again, want 0", len(got))
	}
}
