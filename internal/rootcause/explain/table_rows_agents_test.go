package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// agentCase builds a kube-system DaemonSet pod crash-looping on n1 while
// two workloads' replicas on n1 fail their probes and their replicas on
// n2 stay healthy. It returns the first failing replica.
func agentCase(f *fixture) inventory.EntityID {
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "cni")
	agent := inventory.CoreID(kube.KindPod, "kube-system", "cni-n1")
	f.add(ds, agent)
	f.relate(agent, inventory.OwnedBy, ds)
	f.relate(agent, inventory.RunsOn, nodes[0])
	f.fail(agent, detection.ModeCrashLoop, failingH, 1, "")
	var first inventory.EntityID
	for _, name := range []string{"api", "web"} {
		pods := f.workload("shop", name, 2, nodes...)
		f.fail(pods[0], detection.ModeProbe, failingH, 2, "")
		if first.Name == "" {
			first = pods[0]
		}
	}
	return first
}

var agentRowCases = []rowCase{
	{row: "node-agent-failing", want: "pod/kube-system/cni-n1",
		build: agentCase},
	{row: "node-agent-named-by-error", want: "pod/kube-system/cni-n1",
		build: func(f *fixture) inventory.EntityID {
			_, nodes := agentOnN1(f)
			f.workload("shop", "web", 1, nodes[0])
			pods := f.workload("shop", "api", 1, nodes[0])
			f.fail(pods[0], detection.ModeCreating, failingH, 2,
				"failed to setup network for sandbox")
			return pods[0]
		}},
}

// A healthy agent explains nothing, and a failing agent beside one
// failing workload is a coincidence.
func TestNodeAgentNeedsFailingAgentAndTwoWorkloads(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "cni")
	agent := inventory.CoreID(kube.KindPod, "kube-system", "cni-n1")
	f.add(ds, agent)
	f.relate(agent, inventory.OwnedBy, ds)
	f.relate(agent, inventory.RunsOn, nodes[0])
	f.fail(agent, detection.ModeCrashLoop, failingH, 1, "")
	pods := f.workload("shop", "api", 2, nodes...)
	f.workload("shop", "web", 1, nodes[0])
	f.fail(pods[0], detection.ModeProbe, failingH, 2, "")

	if c, ok := f.explain().CauseOf(pods[0]); ok && c.Root == agent {
		t.Fatalf("one workload beside the agent must not blame it: %+v", c)
	}
}

// everyNodeAgentCase is agentCase with a DaemonSet of the monitoring
// namespace; it has a pod on every node when onEveryNode is set.
func everyNodeAgentCase(f *fixture, onEveryNode bool) (
	agent, effect inventory.EntityID,
) {
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "monitoring", "shipper")
	f.add(ds)
	agent = inventory.CoreID(kube.KindPod, "monitoring", "shipper-n1")
	f.add(agent)
	f.relate(agent, inventory.OwnedBy, ds)
	f.relate(agent, inventory.RunsOn, nodes[0])
	f.fail(agent, detection.ModeCrashLoop, failingH, 1, "")
	if onEveryNode {
		other := inventory.CoreID(kube.KindPod, "monitoring", "shipper-n2")
		f.add(other)
		f.relate(other, inventory.OwnedBy, ds)
		f.relate(other, inventory.RunsOn, nodes[1])
	}
	for _, name := range []string{"api", "web"} {
		pods := f.workload("shop", name, 2, nodes...)
		f.fail(pods[0], detection.ModeProbe, failingH, 2, "")
		if effect.Name == "" {
			effect = pods[0]
		}
	}
	return agent, effect
}

// A DaemonSet outside kube-system that runs on every node is a node
// agent too.
func TestNodeAgentOutsideKubeSystemRunsOnEveryNode(t *testing.T) {
	f := newFixture(t)
	agent, effect := everyNodeAgentCase(f, true)

	c := requireCause(t, f.explain(), effect, agent.String())
	if c.Row != "node-agent-failing" {
		t.Fatalf("row = %s, want node-agent-failing", c.Row)
	}
}

// A DaemonSet that runs on some nodes only is a workload, not an agent
// of the node: it is not blamed for its neighbours.
func TestNodeAgentOutsideKubeSystemNeedsEveryNode(t *testing.T) {
	f := newFixture(t)
	agent, effect := everyNodeAgentCase(f, false)

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == agent {
		t.Fatalf("a DaemonSet on one node blamed as an agent: %+v", c)
	}
}
