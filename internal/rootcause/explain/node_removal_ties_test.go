package explain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// removeAt deletes id at t0 plus after, as an informer delete does.
func removeAt(f *fixture, id inventory.EntityID, after time.Duration) {
	f.t.Helper()
	_, err := f.model.Apply(inventory.Observation{Kind: inventory.Gone,
		Entity: id, Source: "test", At: t0.Add(after)})
	require.NoError(f.t, err)
	f.changes[id] = append(f.changes[id], inventory.Change{
		Entity: id, At: t0.Add(after), Deleted: true})
}

// placedWaiting adds a pod of the ReplicaSet rs placed on node that
// has waited since t0 plus after: its sandbox never comes up.
func placedWaiting(
	f *fixture, rs, node inventory.EntityID, name string,
	after time.Duration,
) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, rs.Namespace, name)
	f.add(pod)
	f.relate(pod, inventory.OwnedBy, rs)
	f.relate(pod, inventory.RunsOn, node)
	f.findings[pod] = []detection.Finding{{Entity: pod,
		Reason: "PodPending", Mode: detection.ModePending,
		Health: failingH, Since: t0.Add(after)}}
	return pod
}

// drainBesideReplacement removes n1, whose api pod is replaced on n3
// and waits there, and in the same minute n2, a cleanly drained node
// whose web pod moved and runs. cart, placed on n3 too, never ran on
// either removed node and waits as well. It returns the api replacement
// and cart.
func drainBesideReplacement(f *fixture) (api, cart inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	gone := f.workload("shop", "api", 1, nodes[0])[0]
	web := f.workload("shop", "web", 1, nodes[1])[0]
	f.workload("shop", "cart", 0)
	removeAt(f, web, 40*time.Second)
	removeAt(f, nodes[1], time.Minute)
	removeAt(f, gone, time.Minute)
	removeAt(f, nodes[0], time.Minute+10*time.Second)
	rs := func(name string) inventory.EntityID {
		return inventory.CoreID(kube.KindReplicaSet, "shop", name+"-1")
	}
	api = placedWaiting(f, rs("api"), nodes[2], "api-1-new",
		time.Minute+20*time.Second)
	cart = placedWaiting(f, rs("cart"), nodes[2], "cart-1-0",
		time.Minute+20*time.Second)
	return api, cart
}

func TestRemovedNodeTiesOnlyThePodsThatMoved(t *testing.T) {
	f := newFixture(t)
	api, cart := drainBesideReplacement(f)
	v := newView(f.snapshot())
	assert.Equal(t, []hop{{link: LinkSchedules,
		to: inventory.CoreID(kube.KindNode, "", "n1")}},
		v.removedNodeHops(api),
		"the replacement waits for the node its pod ran on, not the "+
			"drained node removed in the same minute")
	assert.Empty(t, v.removedNodeHops(cart),
		"a placed pod that never ran on a removed node is not tied "+
			"to the removal by timing alone")
}

func TestRemovedNodeNeverBlamedForPodsThatNeverRanThere(t *testing.T) {
	f := newFixture(t)
	api, cart := drainBesideReplacement(f)
	e := f.explain()
	requireCause(t, e, api, "node//n1")
	for _, root := range causes(e) {
		assert.NotEqual(t, "node//n2", root,
			"a clean drain with no own effect is never a cause")
	}
	if c, ok := e.CauseOf(cart); ok {
		assert.NotEqual(t, kube.KindNode, c.Root.Kind,
			"cart waits for its own node, not for a removal")
	}
}

// unplaced adds a pod of the ReplicaSet rs that the scheduler cannot
// place since two minutes after t0, for the reason verdict gives.
func unplaced(
	f *fixture, rs inventory.EntityID, name, verdict string,
) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, rs.Namespace, name)
	f.add(pod)
	f.relate(pod, inventory.OwnedBy, rs)
	f.findings[pod] = []detection.Finding{{Entity: pod,
		Mode: "Unschedulable", Health: degradedH,
		Since: t0.Add(2 * time.Minute),
		Evidence: []detection.Evidence{{Label: "scheduler",
			Value: "0/1 nodes are available: 1 " + verdict + "."}}}}
	return pod
}

// twoRemovals removes n1, where api ran, and the empty n2 a minute
// after t0, and returns the nodes.
func twoRemovals(f *fixture) []inventory.EntityID {
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	gone := f.workload("shop", "api", 1, nodes[0])[0]
	f.workload("shop", "etl", 0)
	removeAt(f, gone, time.Minute)
	removeAt(f, nodes[0], time.Minute)
	removeAt(f, nodes[1], time.Minute)
	return nodes
}

// TestRemovedNodeRoomForUnplacedPods: a pod the scheduler cannot place
// for lack of cpu waits for the room any node removed just before had,
// whether or not its workload ran there. For another verdict only the
// node its workload left is tied.
func TestRemovedNodeRoomForUnplacedPods(t *testing.T) {
	rs := func(name string) inventory.EntityID {
		return inventory.CoreID(kube.KindReplicaSet, "shop", name+"-1")
	}
	cases := []struct {
		name, owner, verdict string
		want                 []string
	}{
		{"moved, no cpu", "api", "Insufficient cpu",
			[]string{"node//n1", "node//n2"}},
		{"new, no cpu", "etl", "Insufficient cpu",
			[]string{"node//n1", "node//n2"}},
		{"moved, taint", "api", "node(s) had untolerated taint",
			[]string{"node//n1"}},
		{"new, taint", "etl", "node(s) had untolerated taint", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			twoRemovals(f)
			pod := unplaced(f, rs(tc.owner), tc.owner+"-new", tc.verdict)
			var got []string
			for _, h := range newView(f.snapshot()).removedNodeHops(pod) {
				got = append(got, h.to.String())
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
