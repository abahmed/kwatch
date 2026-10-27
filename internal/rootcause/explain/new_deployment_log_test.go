package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// refusedPostgres is the crash log line of the payments-api repro.
const refusedPostgres = "FATAL: connection refused: postgres://db:5432"

// newDeploymentCase creates payments-api, whose only change is its
// creation, and crashes its container with a log that names the
// database it could not reach.
func newDeploymentCase(f *fixture) inventory.EntityID {
	pods := f.workload("default", "payments-api", 1)
	f.change(inventory.CoreID(kube.KindDeployment, "default",
		"payments-api"), 1)
	effect := containerOf(pods[0])
	f.failError(effect, "CrashLoop", "Error")
	f.observe(effect, map[string]inventory.Value{
		kube.AttrLastErrorLine: inventory.Text(refusedPostgres)})
	return effect
}

// A brand-new Deployment's first rollout is not a change shortly
// before: the connection error in the crash log is not blamed on it.
func TestNewDeploymentIsNotBlamedForConnectionRefused(t *testing.T) {
	f := newFixture(t)
	effect := newDeploymentCase(f)

	e := f.explain()
	if c, ok := e.CauseOf(effect); ok &&
		c.Root.Kind == kube.KindDeployment {
		t.Fatalf("new Deployment blamed: %v; all causes %v",
			c.Root, causes(e))
	}
}

// Deleting a Deployment and creating it again at once is still a first
// rollout: the deletion of the old one is no earlier version.
func TestRecreatedDeploymentIsNotBlamedForConnectionRefused(t *testing.T) {
	f := newFixture(t)
	effect := newDeploymentCase(f)
	deployment := inventory.CoreID(kube.KindDeployment, "default",
		"payments-api")
	f.changes[deployment] = append([]inventory.Change{{
		Entity: deployment, At: t0, Deleted: true}},
		f.changes[deployment]...)

	e := f.explain()
	if c, ok := e.CauseOf(effect); ok && c.Root == deployment {
		t.Fatalf("recreated Deployment blamed; all causes %v", causes(e))
	}
}

// A changed template is still blamed: only a creation is ruled out.
func TestChangedDeploymentStillBlamedForConnectionRefused(t *testing.T) {
	f := newFixture(t)
	effect := newDeploymentCase(f)
	deployment := inventory.CoreID(kube.KindDeployment, "default",
		"payments-api")
	f.changes[deployment] = nil
	f.change(deployment, 1, "spec.template.spec.containers.0.image")

	requireRoot(t, f, effect, deployment.String())
}
