package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestExplainZoneNotBlamedThroughHealthyNode: two nodes of zone-a are
// down, but the crashing pods run on its third, healthy node. Their
// crash is not the zone's: the node between them works.
func TestExplainZoneNotBlamedThroughHealthyNode(t *testing.T) {
	f := newFixture(t)
	zone := f.nodes("zone-a", "n1", "n2", "n3")
	f.nodes("zone-b", "n4")
	f.fail(zone[0], "NotReady", failingH, 1, "")
	f.fail(zone[1], "NotReady", failingH, 1, "")
	var crashing []inventory.EntityID
	for _, name := range []string{"a", "b"} {
		for _, pod := range f.workload("shop", name, 1, zone[2]) {
			f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2,
				"")
			crashing = append(crashing, containerOf(pod))
		}
	}
	e := f.explain()
	for _, effect := range crashing {
		if c, ok := e.CauseOf(effect); ok && c.Root.Kind == "zone" {
			t.Fatalf("%s blamed on %s through a healthy node", effect,
				c.Root)
		}
	}
	requireCause(t, e, zone[0], "zone//zone-a")
}

// TestExplainZoneCoversPodsOfItsFailingNodes: a pod on a failing node
// of the zone is still something the zone explains.
func TestExplainZoneCoversPodsOfItsFailingNodes(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	f.fail(nodes[0], "NotReady", failingH, 1, "")
	f.fail(nodes[1], "NotReady", failingH, 1, "")
	pod := f.workload("shop", "a", 1, nodes[0])[0]
	f.fail(pod, detection.ModeNotReady, failingH, 2, "")
	v := newView(f.snapshot())
	cs := v.candidates(failures(f.findings))
	zone, ok := cs.byID[inventory.CoreID(kube.KindZone, "", "zone-a")]
	if !ok {
		t.Fatal("the zone is not a candidate")
	}
	if _, ok := zone.covers[pod]; !ok {
		t.Fatalf("the zone does not cover %s: %v", pod, zone.direct())
	}
}
