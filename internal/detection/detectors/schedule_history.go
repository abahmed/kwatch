package detectors

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultRepeatedFailures is how many scheduled runs in a row must fail
// before a CronJob counts as failing repeatedly.
const DefaultRepeatedFailures = 3

// missedRunReasons are the CronJob controller's Warning events for start
// times it skipped: TooManyMissedTimes when more than 100 were missed
// (it then stops trying), MissSchedule for one run past its deadline.
var missedRunReasons = map[string]bool{
	"TooManyMissedTimes": true, "MissSchedule": true,
}

// missedRuns reports a CronJob whose controller recently said it missed
// start times.
func missedRuns(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil {
		return detection.Finding{}, false
	}
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		if !note.Warning || !missedRunReasons[note.Reason] {
			continue
		}
		// +1ns: the window is inclusive, so the note is gone just after it.
		ctx.RecheckAfter(note.At.Add(EventWindow).Sub(ctx.Now) +
			time.Nanosecond)
		return detection.Finding{
			Reason: reasons.CronJobMissedRuns, Severity: detection.Warning,
			Health: detection.Failing, Since: note.At,
			Summary: "Scheduled runs were missed (" + note.Reason + ")",
			Evidence: []detection.Evidence{
				{Label: "event", Value: note.Message},
			},
		}, true
	}
	return detection.Finding{}, false
}

// blockedRun reports a due run that did not start because the previous
// run is still active and concurrencyPolicy is Forbid. The controller
// skips such runs with a Normal JobAlreadyActive event, which is never
// ingested, and leaves lastScheduleTime unchanged.
func blockedRun(
	ctx detection.Context, e inventory.Entity, next time.Time,
) (detection.Finding, bool) {
	active, _ := number(e, kube.AttrActive)
	if text(e, kube.AttrConcurrencyPolicy) != "Forbid" || active == 0 {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.CronJobBlocked, Severity: detection.Warning,
		Health: detection.Failing, Since: next,
		Summary: "Scheduled run was skipped: the previous run is still " +
			"active and concurrencyPolicy is Forbid (due " +
			format.Duration(ctx.Now.Sub(next)) + " ago)",
		Evidence: []detection.Evidence{
			{Label: "concurrency policy", Value: "Forbid"},
			{Label: "active jobs", Value: countText(active)},
		},
	}, true
}

// finishedJob is one owned Job that completed or failed.
type finishedJob struct {
	name    string
	started time.Time
	failed  bool
	since   time.Time
	// reason is the Failed condition's reason, such as
	// BackoffLimitExceeded.
	reason string
}

// repeatedFailure reports a CronJob whose latest finished run failed and
// that has failed DefaultRepeatedFailures runs in a row, counted from the
// Jobs it still keeps or, since failedJobsHistoryLimit defaults to 1,
// from the runs scheduled since its last success.
func repeatedFailure(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	resume := resumedAfterSuccess(e)
	jobs := finishedJobs(ctx, e.ID)
	if !resume.IsZero() {
		jobs = startedSince(jobs, resume)
	}
	if len(jobs) == 0 || !jobs[0].failed {
		return detection.Finding{}, false
	}
	streak := 0
	for _, job := range jobs {
		if !job.failed {
			break
		}
		streak++
	}
	// Slots counted across a suspension never became Jobs, so after a
	// resume only the Jobs that ran since count.
	runs, ok := scheduledSinceSuccess(ctx, e)
	if ok && resume.IsZero() && int(runs) > streak {
		streak = int(runs)
	}
	if streak < DefaultRepeatedFailures {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason:   reasons.CronJobRepeatedFailure,
		Severity: detection.Warning, Health: detection.Failing,
		Since: jobs[0].since,
		Summary: "The last " + countText(float64(streak)) +
			" scheduled runs failed",
		Evidence: []detection.Evidence{
			{Label: "latest failed job", Value: jobs[0].name},
			{Label: "failed in a row", Value: countText(float64(streak))},
		},
	}, true
}

// resumedAfterSuccess is when kwatch saw the CronJob resume from a
// suspension that began after its last success, or zero. The run count of
// the inventory starts at the last success, so it also counts the slots
// of the suspension. A CronJob kwatch first saw running has no resume.
func resumedAfterSuccess(e inventory.Entity) time.Time {
	attr, ok := e.Attribute(kube.AttrSuspended)
	if !ok || flag(e, kube.AttrSuspended) ||
		!attr.Since.After(e.FirstSeen) {
		return time.Time{}
	}
	if success := timestamp(e, kube.AttrLastSuccess); success.After(attr.Since) {
		return time.Time{}
	}
	return attr.Since
}

// startedSince keeps the Jobs that started at or after the given time.
func startedSince(jobs []finishedJob, from time.Time) []finishedJob {
	var out []finishedJob
	for _, job := range jobs {
		if !job.started.Before(from) {
			out = append(out, job)
		}
	}
	return out
}

// scheduledSinceSuccess returns how many runs were scheduled since the
// last success, which stands in for the failed Jobs the history limit
// already deleted. It counts schedule slots, so it is trusted only when
// every slot became a Job: with concurrencyPolicy Forbid, or after the
// controller reported missed start times, slots were skipped and the
// retained Jobs are the only honest count.
func scheduledSinceSuccess(
	ctx detection.Context, e inventory.Entity,
) (float64, bool) {
	runs, ok := number(e, kube.AttrRunsSinceSuccess)
	if !ok || text(e, kube.AttrConcurrencyPolicy) == "Forbid" {
		return 0, false
	}
	if _, missed := missedRuns(ctx, e); missed {
		return 0, false
	}
	return runs, true
}

// finishedJobs lists the owned Jobs that finished, newest first.
func finishedJobs(
	ctx detection.Context, cron inventory.EntityID,
) []finishedJob {
	if ctx.Model == nil {
		return nil
	}
	var out []finishedJob
	for _, id := range ctx.Model.Related(
		cron, inventory.OwnedBy, inventory.Incoming,
	) {
		job, ok := ctx.Model.Entity(id)
		if !ok || id.Kind != kube.KindJob {
			continue
		}
		failed, failReason, failedSince := condition(job, "Failed")
		complete, _, _ := condition(job, "Complete")
		if failed != "True" && complete != "True" {
			continue
		}
		out = append(out, finishedJob{
			name: id.Name, started: timestamp(job, kube.AttrStartTime),
			failed: failed == "True", since: failedSince,
			reason: failReason,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].started.Equal(out[j].started) {
			return out[i].started.After(out[j].started)
		}
		return out[i].name > out[j].name
	})
	return out
}
