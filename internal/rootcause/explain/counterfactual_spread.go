package explain

import (
	"fmt"
	"sort"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreReplicas compares the replicas of one workload (the pods of one
// ReplicaSet, StatefulSet or DaemonSet). Failing replicas that share
// one node while the others run healthy elsewhere point at that node,
// so a workload-level cause loses weight. Every replica failing on
// several nodes points at the workload, so such a cause gains a little.
// Nodes, zones and pools are judged by exclusivity and coverage.
func scoreReplicas(v *view, c *candidate) outcome {
	if v.blamesOf(c) == blamesPlace {
		return outcome{}
	}
	groups := v.replicaGroups(c)
	for _, g := range groups {
		if g.nodeLocal() {
			text := fmt.Sprintf("%d failing replicas share node %s while "+
				"%d run healthy on other nodes", len(g.failing),
				g.nodes(g.failing)[0].Name, len(g.healthy))
			v.compareGroup(g, rootcause.ProofReplicasNodeLocal, text)
			return outcome{weight: -NodeLocalPenalty,
				code:  rootcause.ProofReplicasNodeLocal,
				count: len(g.failing), total: len(g.failing) + len(g.healthy),
				text: text}
		}
	}
	for _, g := range groups {
		if g.everywhere() {
			text := fmt.Sprintf("%s fail, on %d nodes",
				replicaCount(len(g.failing)), len(g.nodes(g.failing)))
			v.compareGroup(g, rootcause.ProofReplicasEverywhere, text)
			return outcome{weight: EverywhereWeight,
				code:  rootcause.ProofReplicasEverywhere,
				count: len(g.failing), total: len(g.failing), text: text}
		}
	}
	return outcome{}
}

// replicaGroup is the sampled pods of one owner, split by health, with
// the node each runs on.
type replicaGroup struct {
	effect           inventory.EntityID
	failing, healthy []inventory.EntityID
	onNode           map[inventory.EntityID]inventory.EntityID
}

// replicaGroups groups the failing pods of a candidate's failures with
// their siblings under the same direct owner.
func (v *view) replicaGroups(c *candidate) []replicaGroup {
	var out []replicaGroup
	seen := map[inventory.EntityID]bool{}
	for _, u := range v.failingUnits(c) {
		owner, ok := v.directOwner(u.pods[0])
		if !ok || seen[owner] {
			continue
		}
		seen[owner] = true
		g := replicaGroup{effect: u.effect,
			onNode: map[inventory.EntityID]inventory.EntityID{}}
		for _, pod := range sampleIDs(v.ownedPods(owner)) {
			if v.unitFailing(pod) {
				g.failing = append(g.failing, pod)
			} else if v.healthyPod(pod) {
				g.healthy = append(g.healthy, pod)
			}
			if node, ok := v.nodeOf(pod); ok {
				g.onNode[pod] = node
			}
		}
		out = append(out, g)
	}
	return out
}

// directOwner is the controller that owns the pod itself.
func (v *view) directOwner(pod inventory.EntityID) (inventory.EntityID, bool) {
	owners := v.s.Model.Related(pod, inventory.OwnedBy, inventory.Outgoing)
	if len(owners) == 0 {
		return inventory.EntityID{}, false
	}
	return owners[0], true
}

// nodeOf is the node a pod runs on.
func (v *view) nodeOf(pod inventory.EntityID) (inventory.EntityID, bool) {
	nodes := v.s.Model.Related(pod, inventory.RunsOn, inventory.Outgoing)
	if len(nodes) == 0 {
		return inventory.EntityID{}, false
	}
	return nodes[0], true
}

// nodes lists the distinct nodes of the pods, sorted.
func (g replicaGroup) nodes(pods []inventory.EntityID) []inventory.EntityID {
	set := map[inventory.EntityID]bool{}
	for _, pod := range pods {
		if node, ok := g.onNode[pod]; ok {
			set[node] = true
		}
	}
	out := sortedKeys(set)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// nodeLocal reports failing replicas confined to one node while some
// replica is healthy on another and none is healthy on that node.
func (g replicaGroup) nodeLocal() bool {
	failed := g.nodes(g.failing)
	if len(g.failing) < NodeLocalMinFailing || len(failed) != 1 ||
		len(g.healthy) == 0 {
		return false
	}
	for _, node := range g.nodes(g.healthy) {
		if node == failed[0] {
			return false
		}
	}
	return true
}

// everywhere reports every replica failing, on at least two nodes.
func (g replicaGroup) everywhere() bool {
	return len(g.healthy) == 0 && len(g.failing) >= NodeLocalMinFailing &&
		len(g.nodes(g.failing)) >= 2
}

// compareGroup records a replica comparison for the group's failure.
func (v *view) compareGroup(g replicaGroup, code rootcause.ProofCode,
	text string,
) {
	with := g.effect
	if len(g.healthy) > 0 {
		with = g.healthy[0]
	}
	v.compare(g.effect, Comparison{Code: code, With: with,
		Count: len(g.failing), Text: text})
}

// replicaCount is how many replicas, in words: "both replicas" for two,
// "all 3 replicas" for more, "1 replica" for one.
func replicaCount(n int) string {
	switch n {
	case 1:
		return "1 replica"
	case 2:
		return "both replicas"
	}
	return fmt.Sprintf("all %d replicas", n)
}
