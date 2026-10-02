package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultMissedRun is how late a scheduled run may start before it is
// considered missed.
const DefaultMissedRun = 5 * time.Minute

// Schedule detects CronJobs that missed a run, and suspended CronJobs and
// Jobs. Suspension is usually deliberate, so it is informational.
type Schedule struct{}

// Name implements detection.Detector.
func (Schedule) Name() string { return "schedule" }

// Kinds implements detection.Detector.
func (Schedule) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindCronJob, kube.KindJob}
}

// Detect implements detection.Detector.
func (Schedule) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	out := scheduleFindings(ctx, e)
	if e.ID.Kind != kube.KindCronJob {
		return out
	}
	if f, ok := repeatedFailure(ctx, e); ok {
		out = append(out, f)
	}
	return append(out, classReferences(ctx, e)...)
}

func scheduleFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if flag(e, kube.AttrSuspended) {
		reason := reasons.JobSuspended
		if e.ID.Kind == kube.KindCronJob {
			reason = reasons.CronJobSuspended
		}
		return []detection.Finding{{
			Reason: reason, Severity: detection.Info,
			Since:   valueSince(e, kube.AttrSuspended),
			Summary: "Suspended; it will not run until resumed",
		}}
	}
	if found, ok := scheduleProblemFinding(e); ok {
		return []detection.Finding{found}
	}
	next := timestamp(e, kube.AttrNextRun)
	if e.ID.Kind != kube.KindCronJob {
		return nil
	}
	if found, ok := missedRuns(ctx, e); ok {
		return []detection.Finding{found}
	}
	if next.IsZero() {
		return nil
	}
	deadline := next.Add(DefaultMissedRun)
	if ctx.Now.Before(deadline) {
		ctx.RecheckAfter(deadline.Sub(ctx.Now))
		return nil
	}
	if found, ok := blockedRun(ctx, e, next); ok {
		return []detection.Finding{found}
	}
	return []detection.Finding{{
		Reason: reasons.CronJobNotScheduled, Severity: detection.Warning,
		Since: next,
		Summary: "Scheduled run did not start (due " +
			format.Duration(ctx.Now.Sub(next)) + " ago)",
	}}
}

// scheduleProblemSummaries explains each problem the inventory records.
var scheduleProblemSummaries = map[string]string{
	kube.ScheduleInvalid: "Schedule is not a valid cron expression; " +
		"the CronJob will never run",
	kube.ScheduleBadZone: "Time zone is unknown; the CronJob will never run",
}

func scheduleProblemFinding(e inventory.Entity) (detection.Finding, bool) {
	problem := text(e, kube.AttrScheduleProblem)
	summary, ok := scheduleProblemSummaries[problem]
	if !ok {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason:   reasons.CronJobInvalidSchedule,
		Severity: detection.Warning, Since: valueSince(e, kube.AttrScheduleProblem),
		Summary:  summary,
		Evidence: []detection.Evidence{{Label: "problem", Value: problem}},
	}, true
}
