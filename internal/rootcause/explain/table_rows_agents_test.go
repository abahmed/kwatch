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
	f.fail(pods[0], detection.ModeProbe, failingH, 2, "")

	if c, ok := f.explain().CauseOf(pods[0]); ok && c.Root == agent {
		t.Fatalf("one workload beside the agent must not blame it: %+v", c)
	}
}
