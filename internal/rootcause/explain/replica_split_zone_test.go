package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// groupSplitCase runs four replicas, two on ready nodes of zone-a and
// two on nodes of zone-b (of pool-b, spread over zone-b and zone-c,
// when pool is set)
// whose pods crash. It returns the first crashing pod.
func groupSplitCase(pool bool) func(*fixture) inventory.EntityID {
	return func(f *fixture) inventory.EntityID {
		a := f.nodes("zone-a", "a1", "a2")
		b := f.nodes("zone-b", "b1")
		if pool {
			// The pool spans two zones, so no zone holds every failure.
			b = append(b, f.nodes("zone-c", "b2")...)
		} else {
			b = append(b, f.nodes("zone-b", "b2")...)
		}
		if pool {
			group := inventory.CoreID(kube.KindNodePool, "", "pool-b")
			f.add(group)
			healthy := inventory.CoreID(kube.KindNodePool, "", "pool-a")
			f.add(healthy)
			f.relate(a[0], inventory.PartOf, healthy)
			f.relate(a[1], inventory.PartOf, healthy)
			f.relate(b[0], inventory.PartOf, group)
			f.relate(b[1], inventory.PartOf, group)
		}
		pods := f.workload("shop", "api", 4, a[0])
		f.relate(pods[1], inventory.RunsOn, a[1])
		f.relate(pods[2], inventory.RunsOn, b[0])
		f.relate(pods[3], inventory.RunsOn, b[1])
		f.ready(pods...)
		if pool {
			// The errors name what a node does, as a pool outranks the
			// workload only then.
			for _, pod := range pods[2:] {
				f.fail(pod, detection.ModeNetwork, failingH, 5,
					"no runtime for \"runsc\" is configured")
			}
			return pods[2]
		}
		f.crash(pods[2], pods[3])
		return pods[2]
	}
}

var zoneSplitRowCases = []rowCase{
	{row: "shared-zone", want: "zone//zone-b", build: groupSplitCase(false)},
	{row: "shared-nodepool", want: "nodepool//pool-b",
		build: groupSplitCase(true)},
}

// zoneSplitFixture runs four replicas: two in zone-a (ready) and two in
// zone-b, of which the first failing ones crash.
func zoneSplitFixture(
	t *testing.T, failing ...int,
) (*fixture, []inventory.EntityID) {
	f := newFixture(t)
	a := f.nodes("zone-a", "a1", "a2")
	b := f.nodes("zone-b", "b1", "b2")
	pods := f.workload("shop", "api", 4, a[0])
	f.relate(pods[1], inventory.RunsOn, a[1])
	f.relate(pods[2], inventory.RunsOn, b[0])
	f.relate(pods[3], inventory.RunsOn, b[1])
	f.ready(pods...)
	f.crash(pods[failing[0]], pods[failing[1]])
	return f, pods
}

func TestZoneHoldingEveryFailingReplicaIsCleanSplit(t *testing.T) {
	f, pods := zoneSplitFixture(t, 2, 3)
	v := newView(f.snapshot())
	zone := inventory.CoreID("zone", "", "zone-b")
	if !v.cleanReplicaSplit(zone, []inventory.EntityID{pods[2], pods[3]}) {
		t.Fatal("zone-b holds both failing replicas: want a clean split")
	}
}

func TestZoneSplitNeedsReplicasRunningOutside(t *testing.T) {
	f, pods := zoneSplitFixture(t, 0, 2)
	v := newView(f.snapshot())
	zone := inventory.CoreID("zone", "", "zone-b")
	if v.cleanReplicaSplit(zone, []inventory.EntityID{pods[2]}) {
		t.Fatal("a replica fails in zone-a too: not a split by zone")
	}
}
