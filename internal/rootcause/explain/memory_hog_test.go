package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// oomOn records the kernel's system OOM event on a node: the process it
// killed and a memory pressure finding.
func oomOn(f *fixture, node inventory.EntityID, process string) {
	f.apply(inventory.Observation{Kind: inventory.Noted, Entity: node,
		Note: inventory.Note{At: t0.Add(time.Minute), Source: "kubelet",
			Reason:  "SystemOOM",
			Message: "System OOM encountered, victim process: " + process,
			Warning: true}})
	f.fail(node, detection.ModeMemoryPressure, failingH, 2, "")
}

// memoryHogCase has one workload whose process the kernel killed on two
// nodes in turn; it runs on the second now.
func memoryHogCase(f *fixture) ([]inventory.EntityID, inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("batch", "ingest", 1, nodes[1])
	f.setAttrs(containerOf(pods[0]), attrs{
		kube.AttrImage: inventory.Text("registry.example.com/ingest:9")})
	oomOn(f, nodes[0], "ingest")
	oomOn(f, nodes[1], "ingest")
	return nodes, inventory.CoreID(kube.KindDeployment, "batch", "ingest")
}

func TestMemoryHogMovingBetweenNodesIsTheCause(t *testing.T) {
	f := newFixture(t)
	nodes, hog := memoryHogCase(f)
	for _, node := range nodes {
		c := requireCause(t, f.explain(), node, hog.String())
		if c.Row != "workload-exhausts-memory" {
			t.Fatalf("row = %s, want workload-exhausts-memory", c.Row)
		}
	}
}

func TestMemoryHogOnOneNodeLeavesTheNodeBlamed(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("batch", "ingest", 1, nodes[0])
	f.setAttrs(containerOf(pods[0]), attrs{
		kube.AttrImage: inventory.Text("registry.example.com/ingest:9")})
	oomOn(f, nodes[0], "ingest")

	requireCause(t, f.explain(), nodes[0], nodes[0].String())
}

var memoryHogRowCases = []rowCase{
	{row: "workload-exhausts-memory", want: "deployment/batch/ingest",
		build: func(f *fixture) inventory.EntityID {
			nodes, _ := memoryHogCase(f)
			return nodes[1]
		}},
}
