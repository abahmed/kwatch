package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ownerJobComplete reports a pod owned by a Job that finished
// successfully. A retried pod that failed stays in the API until the Job
// is cleaned up, but the Job succeeded, so the failure is history. While
// the Job still runs, or when it failed, the pod stays reported.
func ownerJobComplete(ctx detection.Context, pod inventory.Entity) bool {
	if ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(
		pod.ID, inventory.OwnedBy, inventory.Outgoing,
	) {
		if id.Kind != kube.KindJob {
			continue
		}
		job, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if status, _, _ := condition(job, "Complete"); status == "True" {
			return true
		}
	}
	return false
}
