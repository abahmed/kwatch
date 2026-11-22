package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// PullSplitPenalty lowers a registry blamed for pulls that fail only on
// the nodes of one zone or pool while a replica of the same workload
// outside it runs.
const PullSplitPenalty = 0.3

// scorePullSplit is negative evidence for a registry: when every
// failing pull is on nodes of one zone or pool and the workload's
// replicas on other nodes run, the registry serves them too, so what
// differs is where the pods run, not the registry.
func scorePullSplit(v *view, c *candidate) outcome {
	group, ok := v.pullSplit(c)
	if !ok {
		return outcome{}
	}
	return outcome{weight: -PullSplitPenalty,
		code: rootcause.ProofReplicasNodeLocal,
		text: "the workload's replicas on nodes outside the " +
			string(group.Kind) + " run"}
}

// pullSplit is the zone or pool that holds every pull a registry
// candidate is blamed for, while the workload's replicas outside it run.
func (v *view) pullSplit(c *candidate) (inventory.EntityID, bool) {
	none := inventory.EntityID{}
	if c.id.Kind != kube.KindRegistry {
		return none, false
	}
	var group inventory.EntityID
	effects := c.others()
	for _, effect := range effects {
		pod, ok := v.podOf(effect)
		if !ok {
			return none, false
		}
		node, ok := v.nodeOf(pod)
		if !ok {
			return none, false
		}
		g := v.groupOf(node)
		if g == none || group != none && g != group {
			return none, false
		}
		group = g
	}
	if group == none || !v.runsOutside(effects, group) {
		return none, false
	}
	return group, true
}

// groupOf is the zone of a node, else its pool, or the zero ID.
func (v *view) groupOf(node inventory.EntityID) inventory.EntityID {
	var pool inventory.EntityID
	for _, g := range v.s.Model.Related(node, inventory.PartOf,
		inventory.Outgoing) {
		switch g.Kind {
		case kube.KindZone:
			if g.Name != "" {
				return g
			}
		case kube.KindNodePool:
			pool = g
		}
	}
	return pool
}

// runsOutside reports that every effect has a replica on a node outside
// group that is not failing.
func (v *view) runsOutside(
	effects []inventory.EntityID, group inventory.EntityID,
) bool {
	for _, effect := range effects {
		ok := false
		for _, s := range v.siblings(effect) {
			at, on := v.nodeOf(s)
			if on && !v.inGroup(at, group) && !v.unitFailing(s) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
