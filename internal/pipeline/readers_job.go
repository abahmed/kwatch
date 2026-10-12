package pipeline

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// jobRuns are the pods of an observed Job and their containers. Their
// detectors read the Job: a failed run of a Job that then completed is
// history, so its findings must clear when the Job completes.
func (e *Engine) jobRuns(job inventory.EntityID) []inventory.EntityID {
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
