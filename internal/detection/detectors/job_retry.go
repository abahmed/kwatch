package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// defaultBackoffLimit is how many failed pods a Job may have before it
// fails when its spec names no limit (spec.backoffLimit).
const defaultBackoffLimit = 6

// jobRetrying reports a pod of a Job that is still retrying: the Job
// has neither completed nor failed and has spent less than half of its
// retries. The Job controller replaces a failed pod itself, so such a
// failure is advice. It becomes a finding once half the retries are
// gone, or when the Job fails.
func jobRetrying(ctx detection.Context, pod inventory.Entity) bool {
	if ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(
		pod.ID, inventory.OwnedBy, inventory.Outgoing,
	) {
		if job, ok := ctx.Model.Entity(id); ok && id.Kind == kube.KindJob {
			return hasRetriesLeft(job)
		}
	}
	return false
}

// hasRetriesLeft is a Job that is not over and has failed fewer pods
// than half of its backoff limit, rounded up.
func hasRetriesLeft(job inventory.Entity) bool {
	for _, ending := range []string{"Complete", "Failed"} {
		if status, _, _ := condition(job, ending); status == "True" {
			return false
		}
	}
	failed, _ := number(job, kube.AttrFailed)
	limit, known := number(job, kube.AttrBackoffLimit)
	if !known {
		limit = defaultBackoffLimit
	}
	return int(failed) < (int(limit)+1)/2
}
