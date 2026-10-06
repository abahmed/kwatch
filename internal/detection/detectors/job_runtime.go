package detectors

import (
	"fmt"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Limits for a Job that runs far longer than the CronJob's usual run.
const (
	// longRunFactor is how many times the longest recent success a run
	// may last.
	longRunFactor = 3
	// longRunFloor is the least a run must last to be reported, so a
	// CronJob of five-second runs does not report a one-minute run.
	longRunFloor = 30 * time.Minute
	// minRecentRuns is how many recent successes make "usual" mean
	// something.
	minRecentRuns = 3
	// maxRecentRuns bounds how many successes are compared; a CronJob
	// keeps only a few of them anyway.
	maxRecentRuns = 5
)

// runningLong reports a Job of a CronJob that has run far longer than
// the CronJob's recent successful runs took. Nothing is wrong with the
// Job yet, but it is probably stuck. A Job whose activeDeadlineSeconds
// ends it before this point is left to that deadline: when it expires,
// the failure is reported.
func runningLong(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	started := timestamp(e, kube.AttrStartTime)
	if ctx.Model == nil || started.IsZero() || !jobIsRunning(e) {
		return detection.Finding{}, false
	}
	runs := recentSuccesses(ctx, e.ID)
	if len(runs) < minRecentRuns {
		return detection.Finding{}, false
	}
	slowest := longest(runs)
	limit := max(longRunFactor*slowest, longRunFloor)
	if deadline, ok := number(e, kube.AttrActiveDeadline); ok &&
		time.Duration(deadline*float64(time.Second)) <= limit {
		return detection.Finding{}, false
	}
	if !sustained(ctx, "job-running-long", started, limit) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.JobRunningLong, Severity: detection.Warning,
		Since:   started.Add(limit),
		Summary: "Job has run far longer than its recent runs",
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceRunningFor,
				Value: format.Duration(ctx.Now.Sub(started))},
			{Label: detection.EvidenceRecentRuns, Value: runRange(runs)},
		},
	}, true
}

// jobIsRunning is a Job with pods running that has neither completed
// nor failed and is not suspended.
func jobIsRunning(e inventory.Entity) bool {
	active, _ := number(e, kube.AttrActive)
	for _, ending := range []string{"Complete", "Failed"} {
		if status, _, _ := condition(e, ending); status == "True" {
			return false
		}
	}
	return active > 0 && !flag(e, kube.AttrSuspended)
}

// recentSuccesses are the run times of the newest successful Jobs of
// the CronJob that owns job, as far as the cluster still keeps them.
func recentSuccesses(
	ctx detection.Context, job inventory.EntityID,
) []time.Duration {
	type run struct {
		finished time.Time
		took     time.Duration
	}
	var runs []run
	for _, cron := range ctx.Model.Related(job, inventory.OwnedBy,
		inventory.Outgoing) {
		if cron.Kind != kube.KindCronJob {
			continue
		}
		for _, id := range ctx.Model.Related(cron, inventory.OwnedBy,
			inventory.Incoming) {
			sibling, ok := ctx.Model.Entity(id)
			if !ok || id == job {
				continue
			}
			if took, finished, ok := successRun(sibling); ok {
				runs = append(runs, run{finished, took})
			}
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].finished.After(runs[j].finished)
	})
	var out []time.Duration
	for _, r := range runs[:min(len(runs), maxRecentRuns)] {
		out = append(out, r.took)
	}
	return out
}

// successRun is how long a completed Job took, and when it finished.
func successRun(e inventory.Entity) (time.Duration, time.Time, bool) {
	if status, _, _ := condition(e, "Complete"); status != "True" {
		return 0, time.Time{}, false
	}
	started := timestamp(e, kube.AttrStartTime)
	finished := timestamp(e, kube.AttrCompletionTime)
	if started.IsZero() || finished.IsZero() || !finished.After(started) {
		return 0, time.Time{}, false
	}
	return finished.Sub(started), finished, true
}

func longest(runs []time.Duration) time.Duration {
	var out time.Duration
	for _, took := range runs {
		out = max(out, took)
	}
	return out
}

// runRange writes the shortest and longest run: "8–12m" when both are
// under an hour and over a minute, else "45s–2h10m".
func runRange(runs []time.Duration) string {
	shortest, slowest := runs[0], runs[0]
	for _, took := range runs {
		shortest, slowest = min(shortest, took), max(slowest, took)
	}
	if shortest >= time.Minute && slowest < time.Hour {
		return fmt.Sprintf("%d–%dm", int(shortest.Round(time.Minute).
			Minutes()), int(slowest.Round(time.Minute).Minutes()))
	}
	return format.Duration(shortest) + "–" + format.Duration(slowest)
}
