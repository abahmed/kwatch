package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ownerJobComplete reports a pod owned by a Job that finished
// successfully, or by a failed run a later run of its CronJob made
// history (see runSuperseded). A retried pod that failed stays in the
// API until the Job is cleaned up, but the Job succeeded, so the
// failure is history. While
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
		if status, _, _ := condition(job, "Complete"); status == "True" ||
			runSuperseded(ctx, job) {
			return true
		}
	}
	return false
}

// runSuperseded reports a failed Job of a CronJob that has succeeded
// since: a later run did the work, so the failed run, its pods and
// their containers are history, like a failed pod of a Job that then
// completed. Kubernetes keeps the failed Job in the API until the next
// failure replaces it (failedJobsHistoryLimit), which can be weeks. A
// failure without a time is never superseded.
func runSuperseded(ctx detection.Context, job inventory.Entity) bool {
	status, _, failedAt := condition(job, "Failed")
	if status != "True" || failedAt.IsZero() || ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(
		job.ID, inventory.OwnedBy, inventory.Outgoing,
	) {
		cron, ok := ctx.Model.Entity(id)
		if ok && id.Kind == kube.KindCronJob &&
			timestamp(cron, kube.AttrLastSuccess).After(failedAt) {
			return true
		}
	}
	return false
}
