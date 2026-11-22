package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// cleanReplicaSplit reports a node that splits one workload's replicas
// cleanly: every replica of the same ReplicaSet (or revision) on this
// node fails and every one on other nodes runs. The replicas share a
// template, so what differs between them is where they run.
func (v *view) cleanReplicaSplit(
	node inventory.EntityID, effects []inventory.EntityID,
) bool {
	if node.Kind != kube.KindNode && !isTopology(node) ||
		len(effects) == 0 {
		return false
	}
	// A group is only blamed over its nodes when the failures sit on
	// several of them: one node's failures are that node's.
	if isTopology(node) && v.nodesOf(effects) < 2 {
		return false
	}
	for _, effect := range effects {
		if !v.splitOnNode(effect, node) {
			return false
		}
	}
	return true
}

// splitOnNode reports an effect whose revision siblings on other nodes
// all run, with at least one such sibling.
func (v *view) splitOnNode(effect, node inventory.EntityID) bool {
	pod, ok := v.podOf(effect)
	if !ok {
		return false
	}
	revision := v.s.Model.Related(pod, inventory.OwnedBy, inventory.Outgoing)
	if len(revision) == 0 || revision[0].Kind != kube.KindReplicaSet {
		return false
	}
	elsewhere := 0
	for _, sibling := range rootcause.OwnedPods(v.s.Model, revision[0]) {
		if sibling == pod {
			continue
		}
		at, onNode := v.nodeOf(sibling)
		switch {
		case !onNode:
			continue
		case at == node || v.inGroup(at, node):
			// A sibling that runs here beside the failing ones fits a
			// node-local error too (the kubelet rejects some pods and
			// not others); nodeLocalErrors gates that.
			continue
		case v.unitFailing(sibling):
			return false
		default:
			elsewhere++
		}
	}
	return elsewhere > 0
}

// inGroup reports whether node belongs to the zone or pool group.
func (v *view) inGroup(node, group inventory.EntityID) bool {
	if !isTopology(group) {
		return false
	}
	return slices.Contains(v.s.Model.Related(node, inventory.PartOf,
		inventory.Outgoing), group)
}

// splitAlone reports a node candidate that splits its effects cleanly
// and that nothing else explains: no other candidate, besides a
// workload's own row, covers any of them. A cause with its own evidence
// (an image for another architecture, a release) outranks the bare
// comparison of replicas.
func (v *view) splitAlone(
	cs *candidateSet, c *candidate, effects []inventory.EntityID,
) bool {
	if v.workloadsOf(effects) != 1 || !v.cleanReplicaSplit(c.id, effects) ||
		!isTopology(c.id) && !v.nodeLocalErrors(effects) {
		return false
	}
	for id, other := range cs.byID {
		// A shared error signature of one workload is pruned later for
		// want of a second workload: it explains nothing yet.
		if id == c.id || other.isSelf() || v.pullSplitBy(other, c.id) ||
			v.inGroup(id, c.id) || v.needsMoreWorkloads(other) ||
			isTopology(id) && !v.cleanReplicaSplit(id, effects) ||
			(id.Kind == KindFailureSignature &&
				v.workloadsOf(other.direct()) < SharedMinWorkloads) {
			continue
		}
		for _, effect := range effects {
			if _, covered := other.covers[effect]; covered {
				return false
			}
		}
	}
	return true
}

// nodeLocalMarkers are words of errors that the node a pod runs on
// produces: its network namespace, kernel settings, image store, kubelet
// admission and resource limits. A pod that fails with one while its
// siblings elsewhere run has met the node, not the program.
var nodeLocalMarkers = []string{
	"address already in use", "max_map_count", "failed to register layer",
	"applylayer", "layer is corrupt", "digest mismatch",
	"didn't have enough resource", "outofcpu", "outofmemory",
	"too many open files", "inotify", "no space left on device",
	"hostpath type check failed", "topology affinity",
	"topology locality", "failed to reserve container name",
	"invalid cni configuration", "invalid network config",
	"no networks found in", "cni plugin not initialized",
	"no runtime for", "runtimehandler", "failed to resolve rootfs",
}

// nodeLocalErrors reports whether some effect's own error text carries a
// node-local marker (the pod's own unready finding says nothing of why).
func (v *view) nodeLocalErrors(effects []inventory.EntityID) bool {
	for _, effect := range effects {
		text := strings.ToLower(v.text(effect))
		for _, marker := range nodeLocalMarkers {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}

// pullSplitBy reports a registry candidate that the zone or pool group
// outweighs: its pulls fail only inside group, which runs elsewhere.
func (v *view) pullSplitBy(other *candidate, group inventory.EntityID) bool {
	g, ok := v.pullSplit(other)
	return ok && g == group
}

// needsMoreWorkloads reports a candidate whose every row wants failures
// in more workloads than it has: applyMinWorkloads drops it later, so
// it explains nothing yet.
func (v *view) needsMoreWorkloads(c *candidate) bool {
	groups := effectsNeedingWorkloads(c)
	for _, g := range groups {
		counted := g.effects
		if g.inferred {
			counted = c.direct()
		}
		if v.workloadsOf(counted) >= g.need {
			return false
		}
	}
	return len(groups) > 0
}

// nodesOf counts the distinct nodes the effects' pods run on.
func (v *view) nodesOf(effects []inventory.EntityID) int {
	nodes := map[inventory.EntityID]bool{}
	for _, effect := range effects {
		if pod, ok := v.podOf(effect); ok {
			if at, on := v.nodeOf(pod); on {
				nodes[at] = true
			}
		}
	}
	return len(nodes)
}
