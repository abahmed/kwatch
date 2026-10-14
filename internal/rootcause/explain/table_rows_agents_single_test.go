package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// agentOnN1 adds a failing kube-system agent pod on n1 and returns it
// with the two nodes.
func agentOnN1(f *fixture) (inventory.EntityID, []inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "cni")
	agent := inventory.CoreID(kube.KindPod, "kube-system", "cni-n1")
	f.add(ds, agent)
	f.relate(agent, inventory.OwnedBy, ds)
	f.relate(agent, inventory.RunsOn, nodes[0])
	f.fail(agent, detection.ModeCrashLoop, failingH, 1, "")
	return agent, nodes
}

func blamesAgent(f *fixture, victim, agent inventory.EntityID) bool {
	c, ok := f.explain().CauseOf(victim)
	return ok && c.Root == agent
}

// The only workload on a node with a failing agent is not blamed on it
// without words that name the agent's job: nothing links the two.
func TestNodeAgentNotBlamedForTheOnlyWorkloadOnItsNode(t *testing.T) {
	f := newFixture(t)
	agent, nodes := agentOnN1(f)
	pods := f.workload("shop", "api", 1, nodes[0])
	f.fail(pods[0], detection.ModeProbe, failingH, 2, "")

	if blamesAgent(f, pods[0], agent) {
		t.Fatal("a lone workload must not blame an unrelated agent")
	}
}

// One workload whose error names the agent's job is blamed on the
// agent even when other workloads share the node.
func TestNodeAgentBlamedWhenErrorNamesItsFunction(t *testing.T) {
	f := newFixture(t)
	agent, nodes := agentOnN1(f)
	pods := f.workload("shop", "api", 1, nodes[0])
	f.workload("shop", "web", 1, nodes[0])
	f.fail(pods[0], detection.ModeCreating, failingH, 2,
		"failed to setup network for sandbox: network plugin is not ready")

	if !blamesAgent(f, pods[0], agent) {
		t.Fatal("a network plugin error must blame the failing agent")
	}
}

// A generic failure of one of several workloads is not the agent's.
func TestNodeAgentNotBlamedForOneOfSeveralWorkloads(t *testing.T) {
	f := newFixture(t)
	agent, nodes := agentOnN1(f)
	pods := f.workload("shop", "api", 1, nodes[0])
	f.workload("shop", "web", 1, nodes[0])
	f.fail(pods[0], detection.ModeProbe, failingH, 2, "")

	if blamesAgent(f, pods[0], agent) {
		t.Fatal("one failing app of two must not blame the agent")
	}
}

// A healthy agent is never blamed, even beside a lone failing app.
func TestNodeAgentHealthyIsNotBlamedForOneApp(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "cni")
	agent := inventory.CoreID(kube.KindPod, "kube-system", "cni-n1")
	f.add(ds, agent)
	f.relate(agent, inventory.OwnedBy, ds)
	f.relate(agent, inventory.RunsOn, nodes[0])
	pods := f.workload("shop", "api", 1, nodes[0])
	f.fail(pods[0], detection.ModeCrashLoop, failingH, 2,
		"network plugin is not ready")

	if blamesAgent(f, pods[0], agent) {
		t.Fatal("a healthy agent must not be blamed")
	}
}
