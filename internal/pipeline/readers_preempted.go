package pipeline

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// preemptedPods are the pods of a workload that carry a PodPreempted
// finding. That finding holds while the workload is short of replicas,
// which only the workload's own status says, so when the workload is
// observed its preempted pods are judged again: they clear once the
// replicas are back.
func (e *Engine) preemptedPods(
	workload inventory.EntityID,
) []inventory.EntityID {
	if workload.Kind != kube.KindDeployment &&
		workload.Kind != kube.KindStatefulSet {
		return nil
	}
	var out []inventory.EntityID
	for _, pod := range kube.OwnedPods(e.deps.Model, workload) {
		if hasReason(e.findings[pod], reasons.PodPreempted) {
			out = append(out, pod)
		}
	}
	return out
}

// hasReason reports whether one of the findings has the reason.
func hasReason(found []detection.Finding, reason string) bool {
	for _, f := range found {
		if f.Reason == reason {
			return true
		}
	}
	return false
}
