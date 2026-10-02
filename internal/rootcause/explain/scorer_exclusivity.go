package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreExclusivity is negative evidence: it compares the effects with
// their siblings (other replicas of the same workload) that do not
// depend on the candidate. Healthy siblings elsewhere support the
// candidate; siblings failing elsewhere say the problem follows the
// workload. A zone or pool also needs a healthy peer outside it: in a
// single-zone cluster a zone explains nothing a node does not.
func scoreExclusivity(v *view, c *candidate) outcome {
	if isTopology(c.id) && !v.healthyPeerOutside(c.id) {
		return outcome{code: rootcause.ProofNoHealthyPeer,
			veto: "no healthy node outside this " +
				string(c.id.Kind) + " to compare with"}
	}
	if c.isSelf() && len(c.others()) > 0 {
		return v.selfExclusivity(c)
	}
	healthy, failing := v.siblingsOutside(c)
	switch {
	case failing > healthy:
		return outcome{weight: -ExclusivityPenalty,
			code: rootcause.ProofSiblingsFail,
			text: "replicas that do not depend on it fail too"}
	case healthy > 0 && failing == 0:
		return outcome{weight: ExclusivityWeight,
			code: rootcause.ProofSiblingsHealthy,
			text: "replicas that do not depend on it are healthy"}
	}
	return outcome{}
}

// isTopology reports whether id groups nodes.
func isTopology(id inventory.EntityID) bool {
	return id.Kind == kube.KindZone || id.Kind == kube.KindNodePool
}

// healthyPeerOutside reports whether some node outside the group, in
// another group of the same kind, is healthy.
func (v *view) healthyPeerOutside(group inventory.EntityID) bool {
	for _, node := range v.s.Model.Entities(kube.KindNode) {
		if v.failing(node) {
			continue
		}
		for _, other := range v.s.Model.Related(
			node, inventory.PartOf, inventory.Outgoing,
		) {
			if other.Kind == group.Kind && other != group {
				return true
			}
		}
	}
	return false
}

// selfExclusivity supports a workload blamed for itself when nothing
// it depends on explains its failures and none of its nodes is
// Failing: its node, its config and its registry are fine, so the
// workload is what differs. A cordoned (Degraded) node still counts
// as fine.
func (v *view) selfExclusivity(c *candidate) outcome {
	for _, effect := range c.others() {
		if v.explainedBy[effect] || v.nodeFailing(effect) {
			return outcome{}
		}
	}
	return outcome{weight: ExclusivityWeight,
		code: rootcause.ProofNothingUpstream,
		text: "nothing it depends on explains its failures"}
}

// nodeFailing reports whether the effect's pod runs on a node with a
// Failing finding.
func (v *view) nodeFailing(effect inventory.EntityID) bool {
	pod, ok := v.podOf(effect)
	if !ok {
		return false
	}
	for _, node := range v.s.Model.Related(
		pod, inventory.RunsOn, inventory.Outgoing,
	) {
		for _, f := range v.s.Findings[node] {
			if f.Health == detection.Failing {
				return true
			}
		}
	}
	return false
}

// siblingsOutside counts the healthy and failing siblings of the
// candidate's effects that do not reach the candidate upstream.
func (v *view) siblingsOutside(c *candidate) (healthy, failing int) {
	effects := c.others()
	if len(effects) > ExclusivitySample {
		effects = sampleIDs(effects)[:ExclusivitySample]
	}
	seen := map[inventory.EntityID]bool{}
	for _, effect := range effects {
		for _, sibling := range v.siblings(effect) {
			if seen[sibling] || v.reaches(sibling, c.id) {
				continue
			}
			seen[sibling] = true
			if v.unitFailing(sibling) {
				failing++
			} else {
				healthy++
			}
		}
	}
	return healthy, failing
}

// siblings lists a sample of the other pods of the effect's workload.
func (v *view) siblings(effect inventory.EntityID) []inventory.EntityID {
	pod, ok := v.podOf(effect)
	if !ok {
		return nil
	}
	top := rootcause.TopOwner(v.s.Model, pod)
	if top == pod {
		return nil
	}
	var out []inventory.EntityID
	for _, sibling := range rootcause.OwnedPods(v.s.Model, top) {
		if sibling != pod {
			out = append(out, sibling)
		}
	}
	if len(out) > ExclusivitySample {
		out = sampleIDs(out)[:ExclusivitySample]
	}
	return out
}

// reaches reports whether target is upstream of id.
func (v *view) reaches(id, target inventory.EntityID) bool {
	for _, r := range v.walk(id) {
		if r.id == target {
			return true
		}
	}
	return false
}
