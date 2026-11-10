package pipeline

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// jobRuns are the pods of an observed Job and their containers. Their
// detectors read the Job: a failed run of a Job that then completed is
// history, so its findings must clear when the Job completes.
//
// A CronJob's Jobs read it too: a failed run is history once the
// CronJob records a later success, so an observed CronJob has its Jobs
// and their runs judged again.
func (e *Engine) jobRuns(job inventory.EntityID) []inventory.EntityID {
	if job.Kind == kube.KindCronJob {
		var out []inventory.EntityID
		for _, owned := range e.deps.Model.Related(
			job, inventory.OwnedBy, inventory.Incoming,
		) {
			out = append(out, owned)
			out = append(out, e.jobRuns(owned)...)
		}
		return out
	}
	if job.Kind != kube.KindJob {
		return nil
	}
	var out []inventory.EntityID
	for _, pod := range e.deps.Model.Related(
		job, inventory.OwnedBy, inventory.Incoming,
	) {
		out = append(out, pod)
		out = append(out, e.deps.Model.Related(
			pod, inventory.PartOf, inventory.Incoming)...)
	}
	return out
}
