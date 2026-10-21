package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const gibibyte = float64(1 << 30)

// evictedOn adds a pod of ns/name on node that the kubelet evicted for
// ephemeral storage, with the last usage it reported.
func evictedOn(
	f *fixture, node inventory.EntityID, name string, gi float64,
) inventory.EntityID {
	pods := f.workload("media", name, 1, node)
	f.observe(pods[0], map[string]inventory.Value{
		kube.AttrReason:        inventory.Text("Evicted"),
		kube.AttrPhase:         inventory.Text("Failed"),
		kube.AttrEphemeralUsed: inventory.Number(gi * gibibyte)})
	f.fail(pods[0], detection.ModeEvicted, failingH, 3, "ephemeral-storage")
	return pods[0]
}

// pressure gives a node a disk pressure finding.
func (f *fixture) pressure(node inventory.EntityID) {
	f.fail(node, detection.ModeDiskPressure, failingH, 2, "")
}

// hogCase puts one workload's evicted pods on two nodes under pressure
// and returns the nodes and the workload.
func hogCase(f *fixture) ([]inventory.EntityID, inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2")
	evictedOn(f, nodes[0], "ingest", 41)
	f.pressure(nodes[0])
	f.pressure(nodes[1])
	// The same workload's second pod, on the second node.
	pod := inventory.CoreID(kube.KindPod, "media", "ingest-1-1")
	f.add(pod)
	f.relate(pod, inventory.OwnedBy,
		inventory.CoreID(kube.KindReplicaSet, "media", "ingest-1"))
	f.relate(pod, inventory.RunsOn, nodes[1])
	f.observe(pod, map[string]inventory.Value{
		kube.AttrReason:        inventory.Text("Evicted"),
		kube.AttrPhase:         inventory.Text("Failed"),
		kube.AttrEphemeralUsed: inventory.Number(40 * gibibyte)})
	f.fail(pod, detection.ModeEvicted, failingH, 4, "ephemeral-storage")
	return nodes, inventory.CoreID(kube.KindDeployment, "media", "ingest")
}

// One workload evicted for the disk of two nodes, one after the other,
// is the cause of the pressure on both.
func TestDiskHogMovingBetweenNodesIsTheCause(t *testing.T) {
	f := newFixture(t)
	nodes, hog := hogCase(f)

	e := f.explain()
	for _, node := range nodes {
		c := requireCause(t, e, node, hog.String())
		if c.Row != "workload-fills-disks" {
			t.Fatalf("row = %s, want workload-fills-disks", c.Row)
		}
	}
}

// A hog seen on one node only stays the node's incident: nothing moved.
func TestDiskHogOnOneNodeLeavesTheNodeBlamed(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	evictedOn(f, nodes[0], "ingest", 41)
	f.pressure(nodes[0])

	requireCause(t, f.explain(), nodes[0], nodes[0].String())
}

// A workload that is not the one using most of the disk is not blamed.
func TestDiskHogNeedsMostOfTheEphemeralUse(t *testing.T) {
	f := newFixture(t)
	nodes, hog := hogCase(f)
	other := f.workload("media", "thumbs", 1, nodes[0])[0]
	f.observe(other, map[string]inventory.Value{
		kube.AttrEphemeralUsed: inventory.Number(300 * gibibyte)})

	if c, ok := f.explain().CauseOf(nodes[0]); ok && c.Root == hog {
		t.Fatalf("a workload using a minority of the disk blamed: %+v", c)
	}
}

var diskRowCases = []rowCase{
	{row: "workload-fills-disks", want: "deployment/media/ingest",
		build: func(f *fixture) inventory.EntityID {
			nodes, _ := hogCase(f)
			return nodes[1]
		}},
}

// A hog is blamed for filling disks, not because the workloads that
// fail behind it meet there: the failures are pods evicted from each
// node's disk, and the proof would claim failing workloads depend on it.
func TestDiskHogIsNotProvedByWorkloadsMeetingAtIt(t *testing.T) {
	f := newFixture(t)
	nodes, hog := hogCase(f)
	// Two more workloads fail on the pressured nodes, so that the
	// shared-cause proof would otherwise apply.
	for _, name := range []string{"api", "thumbs"} {
		pods := f.workload("media", name, 1, nodes...)
		f.fail(pods[0], detection.ModeProbe, failingH, 3, "")
	}

	c := requireCause(t, f.explain(), nodes[0], hog.String())
	filled := false
	for _, contribution := range c.Contributions {
		if contribution.Code == rootcause.ProofWorkloadsMeet {
			t.Fatalf("a disk hog proved by failing workloads meeting "+
				"at it: %+v", contribution)
		}
		filled = filled || contribution.Code == rootcause.ProofFillsDisks
	}
	if !filled {
		t.Fatalf("a disk hog lacks its own proof: %+v", c.Contributions)
	}
}
