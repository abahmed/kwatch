package investigate

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var ipNode = inventory.CoreID(kube.KindNode, "", "node-a")

// failedRunModel is one pod whose container failed with containerAttrs.
func failedRunModel(
	t *testing.T, containerAttrs map[string]inventory.Value,
) *inventory.Model {
	t.Helper()
	return testModel(t,
		observed(crashPod, nil),
		observed(crashContainer, containerAttrs),
		related(crashContainer, inventory.PartOf, crashPod))
}

// confirmedFor is the proof an incident of root, with the container
// failing in mode as a member, gets from model.
func confirmedFor(
	t *testing.T, model *inventory.Model, root inventory.EntityID,
	mode string,
) string {
	t.Helper()
	p := incidentOf(root, finding(root, "NodePodIPExhausted"),
		finding(crashContainer, mode))
	p.Cause = &rootcause.CauseRecord{Root: root}
	return confirm(model, p, nil, signature{})
}

var failedRuns = []struct {
	name, mode string
	attrs      map[string]inventory.Value
}{
	{"crash", "Error", map[string]inventory.Value{
		kube.AttrLastExitCode: num(1)}},
	{"oom", "OOMKilled", map[string]inventory.Value{
		kube.AttrLastReason:   text("OOMKilled"),
		kube.AttrLastExitCode: num(137)}},
	{"waiting", "ImagePullBackOff", map[string]inventory.Value{
		kube.AttrStateReason: text("ImagePullBackOff")}},
}

// A node out of pod IPs is not confirmed by an unrelated pod that
// crashed, ran out of memory or could not pull its image.
func TestConfirmIgnoresUnrelatedRunsUnderAnInfrastructureRoot(
	t *testing.T,
) {
	for _, run := range failedRuns {
		t.Run(run.name, func(t *testing.T) {
			model := failedRunModel(t, run.attrs)
			if got := confirmedFor(t, model, ipNode, run.mode); got != "" {
				t.Fatalf("confirmed = %q, want none", got)
			}
		})
	}
}

// The same failed runs confirm an incident whose root is the workload
// that runs them.
func TestConfirmAcceptsRunsUnderAnApplicationRoot(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	for _, run := range failedRuns {
		t.Run(run.name, func(t *testing.T) {
			model := failedRunModel(t, run.attrs)
			if got := confirmedFor(t, model, root, run.mode); got == "" {
				t.Fatal("confirmed = \"\", want a proof")
			}
		})
	}
}

// A pod that is itself the cause's root still proves an infrastructure
// incident.
func TestConfirmAcceptsTheBlamedPodUnderAnInfrastructureRoot(
	t *testing.T,
) {
	model := failedRunModel(t, failedRuns[0].attrs)
	p := incidentOf(ipNode, finding(ipNode, "NodePodIPExhausted"),
		finding(crashContainer, "Error"))
	p.Cause = &rootcause.CauseRecord{Root: crashPod}
	if got := confirm(model, p, nil, signature{}); got == "" {
		t.Fatal("confirmed = \"\", want a proof")
	}
}

// A waiting reason that names the image still proves a registry root.
func TestConfirmWaitingAcceptsTheImageItNames(t *testing.T) {
	model := failedRunModel(t, failedRuns[2].attrs)
	root := inventory.CoreID(kube.KindRegistry, "", "registry.example")
	if got := confirmedFor(t, model, root, "ImagePullBackOff"); got == "" {
		t.Fatal("confirmed = \"\", want a proof")
	}
	if got := confirmedFor(t, model, ipNode, "ImagePullBackOff"); got != "" {
		t.Fatalf("node confirmed = %q, want none", got)
	}
}

// A root that runs no code of its own (an Argo Rollout, a Service) is
// still proved by a run of a pod in its owner chain: the pod's owner is
// the root, or the cause's root.
func TestConfirmAcceptsRunsOfPodsTheRootOwns(t *testing.T) {
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-7d9")
	rollout := inventory.NewEntityID("argoproj.io", "rollout", "shop",
		"api")
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	svc := inventory.CoreID(kube.KindService, "shop", "api")
	for _, run := range failedRuns {
		t.Run(run.name, func(t *testing.T) {
			model := testModel(t,
				observed(crashPod, nil),
				observed(crashContainer, run.attrs),
				related(crashContainer, inventory.PartOf, crashPod),
				related(crashPod, inventory.OwnedBy, rs),
				related(rs, inventory.OwnedBy, rollout))
			if got := confirmedFor(t, model, rollout, run.mode); got == "" {
				t.Fatal("rollout root: confirmed = \"\", want a proof")
			}
			p := incidentOf(svc, finding(svc, "ServiceNoEndpoints"),
				finding(crashContainer, run.mode))
			p.Cause = &rootcause.CauseRecord{Root: deploy}
			if got := confirm(model, p, nil, signature{}); got != "" {
				t.Fatalf("unowned cause: confirmed = %q", got)
			}
			model = testModel(t,
				observed(crashPod, nil),
				observed(crashContainer, run.attrs),
				related(crashContainer, inventory.PartOf, crashPod),
				related(crashPod, inventory.OwnedBy, rs),
				related(rs, inventory.OwnedBy, deploy))
			if got := confirm(model, p, nil, signature{}); got == "" {
				t.Fatal("service root: confirmed = \"\", want a proof")
			}
		})
	}
}
