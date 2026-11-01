package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cronRunUpdate is an update about a CronJob whose latest run, a Job it
// started, failed. Events lists which members the update reports.
func cronRunUpdate(withCronJob bool, failure string) string {
	cron := inventory.CoreID(kube.KindCronJob, "ops", "stats-rollup")
	run := inventory.CoreID(kube.KindJob, "ops", "stats-rollup-292510")
	last := detection.Finding{Entity: cron, Reason: "CronJobLastRunFailed",
		Severity: detection.Warning, Since: at(2, 0),
		Summary: "Cron job stats-rollup has a last run that failed"}
	job := detection.Finding{Entity: run, Reason: "JobBackoffLimitExceeded",
		Severity: detection.Warning, Since: at(2, 0),
		Summary: "Job stats-rollup-292510 " + failure}
	events := []incident.Event{{At: at(2, 0), Text: job.Summary,
		Entity: aboutEntity(run)}}
	if withCronJob {
		events = append(events, incident.Event{At: at(2, 0),
			Text: last.Summary, Entity: aboutEntity(cron)})
	}
	p := incident.Incident{ID: "inc-1", Root: cron, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 2,
		Members: members(last, job), Timeline: events}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}
	return Writer{Cluster: "prod-eu-1"}.Write(d, at(3, 0)).Note
}

// A CronJob's run is the CronJob doing its work: the update says the last
// run failed and quotes why, not that the failure is spreading to a Job.
func TestCronJobUpdateSaysItsLastRunFailed(t *testing.T) {
	note := cronRunUpdate(true, "failed after 3 failed pod(s)")

	assert.Contains(t, note, "Its last run, job stats-rollup-292510, "+
		"failed: failed after 3 failed pod(s).")
	assert.NotContains(t, note, "spreading")
}

// The failure text is the detector's own, whatever the count.
func TestCronJobUpdateQuotesTheRunsOwnFailure(t *testing.T) {
	note := cronRunUpdate(true, "failed after 5 failed pod(s)")

	assert.Contains(t, note, "failed: failed after 5 failed pod(s).")
	assert.NotContains(t, note, "3 failed")
}

// When the update has not yet named the CronJob, the sentence does.
func TestCronJobUpdateNamesTheCronJobWhenNothingElseDid(t *testing.T) {
	note := cronRunUpdate(false, "failed after 3 failed pod(s)")

	assert.Contains(t, note, "The last run of cron job stats-rollup in ops "+
		"(prod-eu-1), job stats-rollup-292510, failed: failed after 3 "+
		"failed pod(s).")
}
