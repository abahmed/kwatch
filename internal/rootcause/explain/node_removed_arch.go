package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeRemovedArchRow says that a node removed just before a pod began
// to crash with an image built for another CPU explains it when the
// workload's pods ran on that node until it went: they ran fine there,
// and their replacements landed on nodes of another CPU because the
// removal took the nodes of theirs away (a scale-down of a pool). The
// image's own CPU fault is real but was not new; the removal is.
var nodeRemovedArchRow = Row{
	Name: RuleNodeRemovedArch,
	Cause: Side{Kind: kube.KindNode,
		Modes: []detection.Mode{ModeDeleted}},
	Link:   LinkSchedules,
	Effect: podSide(podFailures...),
	Prior:  0.85,
}

// removedArchNodeHops lead from a pod whose container crashes on a node
// of another CPU to the nodes removed shortly before it began to fail
// that its workload's pods ran on (removalTouches). Without the CPU
// mismatch, timing alone ties no pod to a removal.
func (v *view) removedArchNodeHops(pod inventory.EntityID) []hop {
	if pod.Kind != kube.KindPod || !v.archMismatch(pod) {
		return nil
	}
	began := v.earliestSince(append([]inventory.EntityID{pod},
		v.s.Model.Related(pod, inventory.PartOf, inventory.Incoming)...))
	if !began.ok {
		return nil
	}
	var out []hop
	for _, node := range v.removedOf(kube.KindNode) {
		if _, ok := v.replacedAfter(node, began.t); ok &&
			v.removalTouches(pod, node) {
			out = append(out, hop{link: LinkSchedules, to: node})
		}
	}
	return out
}

// archMismatch reports a pod one of whose containers crashes on a node
// whose CPU its image is not built for.
func (v *view) archMismatch(pod inventory.EntityID) bool {
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		if archEvidence(v.s.Findings[id]) {
			return true
		}
	}
	return false
}
