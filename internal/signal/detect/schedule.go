package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultMissedRun is how late a scheduled run may start before it is
// considered missed.
const DefaultMissedRun = 5 * time.Minute

// Schedule detects CronJobs that missed a run, and suspended CronJobs and
// Jobs. Suspension is usually deliberate, so it is informational.
type Schedule struct{}

// Name implements signal.Detector.
func (Schedule) Name() string { return "schedule" }

// Kinds implements signal.Detector.
func (Schedule) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindCronJob, kube.KindJob}
}

// Detect implements signal.Detector.
func (Schedule) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	if flag(e, kube.AttrSuspended) {
		reason := constant.ReasonJobSuspended
		if e.ID.Kind == kube.KindCronJob {
			reason = constant.ReasonCronJobSuspended
		}
		return []signal.Signal{{
			Reason: reason, Severity: signal.Info,
			Since:   valueSince(e, kube.AttrSuspended),
			Summary: "Suspended; it will not run until resumed",
		}}
	}
	next := timestamp(e, kube.AttrNextRun)
	if e.ID.Kind != kube.KindCronJob || next.IsZero() {
		return nil
	}
	deadline := next.Add(DefaultMissedRun)
	if ctx.Now.Before(deadline) {
		ctx.RecheckAfter(deadline.Sub(ctx.Now))
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonCronJobNotScheduled, Severity: signal.Warning,
		Since: next,
		Summary: "Scheduled run did not start (due " +
			format.Duration(ctx.Now.Sub(next)) + " ago)",
	}}
}
