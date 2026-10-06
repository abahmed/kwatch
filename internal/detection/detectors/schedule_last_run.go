package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// lastRunFailed reports a CronJob whose most recent finished run failed
// and that has not succeeded since: one failed run is worth knowing
// about, not an interruption, so the finding is informational and waits
// for the digest. Three failures in a row are repeatedFailure's, and a
// suspended CronJob is not expected to run. The finding clears by
// itself when a later run succeeds or the failed Job is deleted.
func lastRunFailed(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if flag(e, kube.AttrSuspended) {
		return detection.Finding{}, false
	}
	jobs := finishedJobs(ctx, e.ID)
	if len(jobs) == 0 || !jobs[0].failed {
		return detection.Finding{}, false
	}
	failed := jobs[0]
	if !failed.since.IsZero() &&
		!timestamp(e, kube.AttrLastSuccess).Before(failed.since) {
		return detection.Finding{}, false
	}
	summary := "Last run failed " + ageText(ctx.Now, failed.since)
	if failed.reason != "" {
		summary += " (" + failed.reason + ")"
	}
	if next := timestamp(e, kube.AttrNextRun); !next.IsZero() {
		summary += "; the next run is at " +
			next.UTC().Format("2006-01-02 15:04 MST")
	}
	return detection.Finding{
		Reason: reasons.CronJobLastRunFailed, Severity: detection.Info,
		Since: failed.since, Summary: summary,
		Evidence: []detection.Evidence{
			{Label: "failed job", Value: failed.name},
		},
	}, true
}

// ageText says how long ago something happened: "3 days ago" from two
// days on, "5h12m ago" before. An unknown time is just "recently".
func ageText(now, at time.Time) string {
	if at.IsZero() || at.After(now) {
		return "recently"
	}
	age := now.Sub(at)
	if age >= 48*time.Hour {
		return strconv.Itoa(int(age.Hours()/24)) + " days ago"
	}
	return format.Duration(age) + " ago"
}

// staleRunAge is how old a failed CronJob run must be before it stops
// being news: the CronJob's own last-run finding reports it from then
// on, for the digest.
const staleRunAge = 24 * time.Hour

// failedJobSeverity is a failed Job's severity: a warning, except for a
// run of a CronJob that failed more than staleRunAge ago, which is only
// informational. The failure was news when it happened.
func failedJobSeverity(
	ctx detection.Context, job inventory.Entity, failedAt time.Time,
) detection.Severity {
	if failedAt.IsZero() || ctx.Model == nil || !cronOwned(ctx, job) {
		return detection.Warning
	}
	if age := ctx.Now.Sub(failedAt); age < staleRunAge {
		ctx.RecheckAfter(staleRunAge - age)
		return detection.Warning
	}
	return detection.Info
}

func cronOwned(ctx detection.Context, job inventory.Entity) bool {
	for _, owner := range ctx.Model.Related(
		job.ID, inventory.OwnedBy, inventory.Outgoing) {
		if owner.Kind == kube.KindCronJob {
			return true
		}
	}
	return false
}
