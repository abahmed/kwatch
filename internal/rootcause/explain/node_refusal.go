package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeRefusesPods is the pseudo mode of a node whose own kubelet
// refused a pod at admission for want of a resource it counts itself
// (OutOfcpu, OutOfmemory, OutOfpods): the refusal is about that node.
const ModeRefusesPods detection.Mode = "RefusesPods"

// RuleAdmissionRejectedOnNode names the row for a pod a node's kubelet
// refused at admission.
const RuleAdmissionRejectedOnNode = "admission-rejected-on-node"

// refusalState is ModeRefusesPods for the node a failing pod was
// refused by, and nothing for any other entity.
func (v *view) refusalState(id, effect inventory.EntityID) state {
	if id.Kind != kube.KindNode {
		return state{id: id}
	}
	unit, ok := v.unitOf(effect)
	if !ok || !v.refusedAt(unit) {
		return state{id: id}
	}
	return state{id: id, modes: []modeHealth{{
		mode: ModeRefusesPods, health: detection.Failing, pseudo: true}}}
}

// refusedAt reports a pod with a finding of its kubelet refusing it for
// want of a resource.
func (v *view) refusedAt(pod inventory.EntityID) bool {
	for _, f := range v.s.Findings[pod] {
		if f.Mode.Within(detection.ModeAdmissionRejected) &&
			strings.Contains(string(f.Mode), ".OutOf") {
			return true
		}
	}
	return false
}

// refusedByNode reports that every effect is a pod the node's own
// kubelet refused at admission: the refusal is about that node whatever
// the number of workloads.
func refusedByNode(c *candidate, effects []inventory.EntityID) bool {
	if len(effects) == 0 {
		return false
	}
	for _, effect := range effects {
		if c.covers[effect].match.row.Name != RuleAdmissionRejectedOnNode {
			return false
		}
	}
	return true
}
