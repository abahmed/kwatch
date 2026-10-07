package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// blockingJob is the run of a Forbid CronJob that is still going and
// keeps the later runs from starting.
type blockingJob struct {
	name    string
	started time.Time
	// pod is what the job's pod is stuck in; empty when it looks fine.
	pod string
}

// skippedRuns counts the schedule times that passed since the last run
// was created; each of them found the previous run still active.
func skippedRuns(ctx detection.Context, e inventory.Entity) (int, bool) {
	last := timestamp(e, kube.AttrLastSchedule)
	expr := text(e, kube.AttrSchedule)
	if last.IsZero() || expr == "" {
		return 0, false
	}
	return kube.ScheduleSlots(expr, text(e, kube.AttrTimeZone), last, ctx.Now)
}

// activeJob finds the oldest unfinished Job the CronJob owns.
func activeJob(
	ctx detection.Context, cron inventory.EntityID,
) (blockingJob, bool) {
	if ctx.Model == nil {
		return blockingJob{}, false
	}
	var found blockingJob
	var ok bool
	for _, id := range ctx.Model.Related(
		cron, inventory.OwnedBy, inventory.Incoming,
	) {
		job, exists := ctx.Model.Entity(id)
		if !exists || id.Kind != kube.KindJob || jobEnded(job) {
			continue
		}
		started := timestamp(job, kube.AttrStartTime)
		if !ok || started.Before(found.started) ||
			(started.Equal(found.started) && id.Name < found.name) {
			found = blockingJob{name: id.Name, started: started,
				pod: stuckPodState(ctx.Model, id)}
			ok = true
		}
	}
	return found, ok
}

// jobEnded is a Job that completed or failed.
func jobEnded(job inventory.Entity) bool {
	for _, ending := range []string{"Complete", "Failed"} {
		if status, _, _ := condition(job, ending); status == "True" {
			return true
		}
	}
	return false
}

// stuckPodState names why a Job's pod does not make progress. Empty
// when the pod runs.
func stuckPodState(model inventory.Reader, job inventory.EntityID) string {
	for _, id := range model.Related(job, inventory.OwnedBy,
		inventory.Incoming) {
		pod, ok := model.Entity(id)
		if !ok || id.Kind != kube.KindPod {
			continue
		}
		if state := podTrouble(model, pod); state != "" {
			return state
		}
	}
	return ""
}

// blockedText says which run keeps the CronJob from starting new ones,
// and how many runs it has cost so far.
func blockedText(job blockingJob, skipped int) string {
	out := "Scheduled runs are skipped"
	switch {
	case skipped == 1:
		out = "Skipped 1 scheduled run"
	case skipped > 1:
		out = "Skipped " + countText(float64(skipped)) + " scheduled runs"
	}
	out += ": job " + job.name
	if !job.started.IsZero() {
		out += " from " + job.started.UTC().Format("2006-01-02 15:04 MST")
	}
	out += " is still running"
	if job.pod != "" {
		out += " (pod stuck in " + job.pod + ")"
	}
	return out + " and concurrencyPolicy is Forbid"
}
