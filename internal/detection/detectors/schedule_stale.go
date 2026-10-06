package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultStaleRuns is how many scheduled runs may pass without a success
// before a CronJob is reported. The inventory counts the runs from the
// cron schedule, so a monthly job is reported after three months and a
// job that runs every minute after three minutes; for the fast one the
// failing-run findings speak first.
const DefaultStaleRuns = 3

// noRecentSuccess reports a CronJob that has not succeeded for
// DefaultStaleRuns scheduled runs. It is the digest's reminder of a job
// nobody looks at, such as a monthly backup that has failed since long
// ago, and it needs no failed Job to be left over: the history limit
// deletes them. A suspended CronJob is not expected to run, and one that
// resumed since its last success counts the slots of the suspension, so
// neither is reported.
func noRecentSuccess(
	e inventory.Entity,
) (detection.Finding, bool) {
	runs, ok := number(e, kube.AttrRunsSinceSuccess)
	if !ok || runs < DefaultStaleRuns || flag(e, kube.AttrSuspended) ||
		!resumedAfterSuccess(e).IsZero() {
		return detection.Finding{}, false
	}
	since := timestamp(e, kube.AttrLastSuccess)
	summary := "Has not succeeded since it was created"
	if since.IsZero() {
		since = e.FirstSeen
	} else {
		summary = "Has not succeeded since " + since.UTC().Format(
			time.DateOnly)
	}
	return detection.Finding{
		Reason: reasons.CronJobNoRecentSuccess, Severity: detection.Info,
		Health: detection.Degraded, Since: since,
		Summary: summary + " (" + countText(runs) + " scheduled runs)",
	}, true
}

// hasReason reports whether one of the findings has the reason.
func hasReason(found []detection.Finding, reason string) bool {
	for _, f := range found {
		if f.Reason == reason {
			return true
		}
	}
	return false
}
