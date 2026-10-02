package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreCoverage compares the candidate's failing dependents with all
// of them. A node whose every pod fails is a better cause than one
// where one pod in twenty fails.
func scoreCoverage(v *view, c *candidate) outcome {
	// An entity blamed only for itself is judged by its own finding;
	// what its dependents do says nothing about that.
	if len(c.others()) == 0 {
		return outcome{}
	}
	dependents := sampleIDs(v.dependents(c.id))
	if len(dependents) < CoverageMinDependents {
		return outcome{}
	}
	failing := 0
	for _, id := range dependents {
		if v.unitFailing(id) {
			failing++
		}
	}
	share := float64(failing) / float64(len(dependents))
	return outcome{weight: CoverageWeight * (2*share - 1),
		code: rootcause.ProofDependentsFail, count: failing,
		total: len(dependents), text: fmt.Sprintf("%d of %d dependents fail", failing,
			len(dependents))}
}

// dependents lists what depends on id directly: the pods of a node or
// an owner, the nodes of a zone, the users of a secret. Virtual kinds
// other than zones and pools have no stored dependents.
func (v *view) dependents(id inventory.EntityID) []inventory.EntityID {
	switch id.Kind {
	case kube.KindNode:
		return v.s.Model.Related(id, inventory.RunsOn, inventory.Incoming)
	case kube.KindZone, kube.KindNodePool:
		return v.s.Model.Related(id, inventory.PartOf, inventory.Incoming)
	case kube.KindPod:
		return nil
	}
	if pods := rootcause.OwnedPods(v.s.Model, id); len(pods) > 0 {
		return pods
	}
	var out []inventory.EntityID
	for _, relation := range v.s.Model.Relations(id, inventory.Incoming) {
		if relation.From.Kind == kube.KindPod {
			out = append(out, relation.From)
		}
	}
	return out
}
