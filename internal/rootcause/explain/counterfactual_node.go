package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreNodePeers asks whether the node under the failing pods is fine
// for the other workloads on it. A node that runs several healthy pods
// of other workloads, and none that fail, is not what broke these pods,
// so a cause that blames something else gains a little. A node that
// hosts many failures is judged by coverage and the shared-node rule,
// which keep their own safeguards; this never raises a node.
func scoreNodePeers(v *view, c *candidate) outcome {
	if v.blamesOf(c) == blamesPlace {
		return outcome{}
	}
	units := v.failingUnits(c)
	first, fewest, ok := v.clearedNode(units)
	if !ok {
		return outcome{}
	}
	text := fmt.Sprintf("node %s is healthy for %d other pods",
		first.Name, fewest)
	for _, u := range units {
		v.compare(u.effect, Comparison{Code: rootcause.ProofNodePeersHealthy,
			With: first, Count: fewest, Text: text})
	}
	return outcome{weight: NodePeersWeight, count: fewest,
		code: rootcause.ProofNodePeersHealthy, text: text}
}

// nodePeers counts a sample of the pods of other workloads on a node:
// those that are healthy and those that fail.
func (v *view) nodePeers(
	node inventory.EntityID, failing map[inventory.EntityID]bool,
) (healthy, broken int) {
	for _, pod := range sampleIDs(v.s.Model.Related(
		node, inventory.RunsOn, inventory.Incoming,
	)) {
		if failing[v.workloadOf(pod)] {
			continue
		}
		if v.unitFailing(pod) {
			broken++
		} else if v.healthyPod(pod) {
			healthy++
		}
	}
	return healthy, broken
}

// clearedNode checks the node of every failing pod. It returns the
// first node and the fewest healthy peers any of them has, and ok only
// when every node has NodePeersMin healthy peers, none failing, and no
// finding of its own.
func (v *view) clearedNode(
	units []failingUnit,
) (first inventory.EntityID, fewest int, ok bool) {
	failing := v.workloadsOfUnits(units)
	for _, u := range units {
		for _, pod := range u.pods {
			node, found := v.nodeOf(pod)
			if !found || v.failing(node) {
				return first, 0, false
			}
			healthy, broken := v.nodePeers(node, failing)
			if healthy < NodePeersMin || broken > 0 {
				return first, 0, false
			}
			if !ok || healthy < fewest {
				first, fewest, ok = node, healthy, true
			}
		}
	}
	return first, fewest, ok
}
