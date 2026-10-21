package explain

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const panicText = "panic: runtime error: invalid memory address or nil " +
	"pointer dereference"

// driverPod adds a pod of ds on node whose arguments name driver.
func driverPod(
	f *fixture, ds inventory.EntityID, name string,
	node inventory.EntityID, driver string,
) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, ds.Namespace, name)
	attrs := map[string]inventory.Value{}
	if driver != "" {
		attrs[kube.AttrDriverNames] = inventory.Text(driver)
	}
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
		Attributes: attrs})
	f.relate(pod, inventory.OwnedBy, ds)
	f.relate(pod, inventory.RunsOn, node)
	return pod
}

// agentWithPeers fails a log shipper on n1, where two of six workloads
// crash on their own panics and the others run, each with a replica on
// n2. It returns the agent and the first crashing replica.
func agentWithPeers(f *fixture) (agent, effect inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "shipper")
	f.add(ds)
	agent = driverPod(f, ds, "shipper-n1", nodes[0], "")
	driverPod(f, ds, "shipper-n2", nodes[1], "")
	f.fail(agent, detection.ModeCrashLoop, failingH, 2, "")
	for i := 0; i < 6; i++ {
		pods := f.workload("shop", fmt.Sprintf("app%d", i), 2, nodes...)
		if i >= 2 {
			continue
		}
		f.fail(containerOf(pods[0]), detection.ModeCrashLoop, failingH,
			3, panicText)
		if effect.IsZero() {
			effect = containerOf(pods[0])
		}
	}
	return agent, effect
}

// An agent serves every pod on its node: two crashes among many healthy
// pods are not its doing.
func TestAgentGateMostPodsOnTheNodeAreHealthy(t *testing.T) {
	f := newFixture(t)
	agent, effect := agentWithPeers(f)

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == agent {
		t.Fatalf("a failing agent blamed for two of many pods: %+v", c)
	}
}

// Each failing pod's workload must run a healthy replica elsewhere,
// else it may fail the same way on any node.
func TestAgentGateNeedsAHealthyReplicaElsewhere(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "cni")
	f.add(ds)
	agent := driverPod(f, ds, "cni-n1", nodes[0], "")
	driverPod(f, ds, "cni-n2", nodes[1], "")
	f.fail(agent, detection.ModeCrashLoop, failingH, 1, "")
	var effect inventory.EntityID
	for _, name := range []string{"api", "web"} {
		pods := f.workload("shop", name, 1, nodes[0])
		f.fail(pods[0], detection.ModeProbe, failingH, 2, "")
		effect = pods[0]
	}

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == agent {
		t.Fatalf("workloads with no replica elsewhere blamed on the "+
			"agent: %+v", c)
	}
}

// The failures an agent explains must have begun together.
func TestAgentGateFailuresMustBeginTogether(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "cni")
	f.add(ds)
	agent := driverPod(f, ds, "cni-n1", nodes[0], "")
	driverPod(f, ds, "cni-n2", nodes[1], "")
	f.fail(agent, detection.ModeCrashLoop, failingH, 1, "")
	var effect inventory.EntityID
	for i, name := range []string{"api", "web"} {
		pods := f.workload("shop", name, 2, nodes...)
		f.fail(pods[0], detection.ModeProbe, failingH, 2+i*30, "")
		effect = pods[0]
	}

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == agent {
		t.Fatalf("failures half an hour apart blamed on the agent: %+v", c)
	}
}

const csiMissing = "MountVolume.MountDevice failed for volume \"pvc-1\": " +
	"driver name ebs.csi.example.com not found in the list of " +
	"registered CSI drivers"

// csiCase fails the proxy on n1 and a pod on n1 that cannot mount a
// volume because csiMissing; the CSI node plugin pod on n1 carries
// plugin driver names. It returns the proxy pod, the plugin pod and the
// pod that cannot mount.
func csiCase(
	f *fixture, plugin string,
) (proxy, csi, victim inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2")
	proxyDS := inventory.CoreID(kube.KindDaemonSet, "kube-system", "proxy")
	csiDS := inventory.CoreID(kube.KindDaemonSet, "kube-system", "csi")
	f.add(proxyDS, csiDS)
	proxy = driverPod(f, proxyDS, "proxy-n1", nodes[0], "")
	driverPod(f, proxyDS, "proxy-n2", nodes[1], "")
	csi = driverPod(f, csiDS, "csi-n1", nodes[0], plugin)
	driverPod(f, csiDS, "csi-n2", nodes[1], plugin)
	f.fail(proxy, detection.ModeCrashLoop, failingH, 1, "")
	victim = f.workload("shop", "api", 1, nodes[0])[0]
	f.fail(victim, modeFailedMount, failingH, 2, csiMissing)
	return proxy, csi, victim
}

// A volume error that names a storage driver is not the proxy's fault.
func TestAgentDriverGateOtherAgentIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	proxy, _, victim := csiCase(f, "ebs.csi.example.com")

	if c, ok := f.explain().CauseOf(victim); ok && c.Root == proxy {
		t.Fatalf("a failing proxy blamed for a missing storage "+
			"driver: %+v", c)
	}
}

// The agent that is given the driver's name is the one blamed.
func TestAgentDriverGateBlamesTheRegisteringAgent(t *testing.T) {
	f := newFixture(t)
	_, csi, victim := csiCase(f, "ebs.csi.example.com")
	f.fail(csi, detection.ModeCrashLoop, failingH, 1, "")

	c := requireCause(t, f.explain(), victim, csi.String())
	if c.Row != agentNamedRow {
		t.Fatalf("row = %s, want %s", c.Row, agentNamedRow)
	}
}

// Another agent's arguments do not make it the one the error names.
func TestAgentDriverGateIgnoresAnotherDriversPlugin(t *testing.T) {
	f := newFixture(t)
	_, csi, victim := csiCase(f, "other.csi.example.com")
	f.fail(csi, detection.ModeCrashLoop, failingH, 1, "")

	if c, ok := f.explain().CauseOf(victim); ok && c.Root == csi {
		t.Fatalf("a plugin for another driver blamed: %+v", c)
	}
}

// A DaemonSet that registers the driver on other nodes but has no pod
// on this one is the cause of the missing registration.
func TestAgentAbsentDaemonSetHasNoPodOnTheNode(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "csi")
	f.add(ds)
	driverPod(f, ds, "csi-n1", nodes[0], "ebs.csi.example.com")
	driverPod(f, ds, "csi-n2", nodes[1], "ebs.csi.example.com")
	victim := f.workload("shop", "api", 1, nodes[2])[0]
	f.fail(victim, modeFailedMount, failingH, 2, csiMissing)

	c := requireCause(t, f.explain(), victim, ds.String())
	if c.Row != "node-agent-absent" {
		t.Fatalf("row = %s, want node-agent-absent", c.Row)
	}
}

// A DaemonSet with a pod on the node is not absent from it.
func TestAgentAbsentNotBlamedWhenItsPodRunsThere(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	ds := inventory.CoreID(kube.KindDaemonSet, "kube-system", "csi")
	f.add(ds)
	driverPod(f, ds, "csi-n1", nodes[0], "ebs.csi.example.com")
	driverPod(f, ds, "csi-n2", nodes[1], "ebs.csi.example.com")
	victim := f.workload("shop", "api", 1, nodes[0])[0]
	f.fail(victim, modeFailedMount, failingH, 2, csiMissing)

	if c, ok := f.explain().CauseOf(victim); ok && c.Root == ds {
		t.Fatalf("a DaemonSet with a pod on the node is absent: %+v", c)
	}
}
