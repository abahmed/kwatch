package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// staleCron is a CronJob last succeeded at success, with runs scheduled
// since (the inventory counts them from the cron schedule).
func staleCron(
	m *inventory.Model, success time.Time, runs float64,
) inventory.EntityID {
	cron := newID(kube.KindCronJob, "ns", "postgres-backup")
	attrs := map[string]inventory.Value{
		kube.AttrRunsSinceSuccess: inventory.Number(runs),
	}
	if !success.IsZero() {
		attrs[kube.AttrLastSuccess] = inventory.Time(success)
	}
	put(m, cron, t0, attrs)
	return cron
}

// A monthly CronJob whose last success was 29 runs ago is reported for
// the digest, with the date and the number of runs, even when no failed
// Job is left to count.
func TestScheduleReportsAMonthlyCronJobThatStoppedSucceeding(t *testing.T) {
	m := newTestModel()
	success := time.Date(2024, 5, 20, 15, 33, 19, 0, time.UTC)
	cron := staleCron(m, success, 29)

	eval := evaluate(Schedule{}, m, t0, cron, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, reasons.CronJobNoRecentSuccess, f.Reason)
	assert.Equal(t, detection.Info, f.Severity)
	assert.Equal(t, "Schedule.NoRecentSuccess", string(f.Mode))
	assert.Equal(t, "Has not succeeded since 2024-05-20 (29 scheduled runs)",
		f.Summary)
	assert.True(t, f.Since.Equal(success))
}

func TestScheduleIgnoresAFewSkippedRuns(t *testing.T) {
	m := newTestModel()
	cron := staleCron(m, t0.Add(-60*24*time.Hour), 2)

	assert.Empty(t, evaluate(Schedule{}, m, t0, cron, nil).Findings)
}

func TestScheduleNoRecentSuccessSaysWhenItNeverSucceeded(t *testing.T) {
	m := newTestModel()
	cron := staleCron(m, time.Time{}, 5)

	eval := evaluate(Schedule{}, m, t0, cron, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Has not succeeded since it was created "+
		"(5 scheduled runs)", eval.Findings[0].Summary)
}

// A suspended CronJob is not expected to run, and the repeated-failure
// finding already says what a failing one needs.
func TestScheduleNoRecentSuccessLeavesSuspendedAndFailingToTheirOwn(
	t *testing.T,
) {
	m := newTestModel()
	suspended := staleCron(m, t0.Add(-90*24*time.Hour), 9)
	put(m, suspended, t0, map[string]inventory.Value{
		kube.AttrRunsSinceSuccess: inventory.Number(9),
		kube.AttrSuspended:        inventory.Bool(true),
	})
	for _, f := range evaluate(Schedule{}, m, t0, suspended, nil).Findings {
		assert.NotEqual(t, reasons.CronJobNoRecentSuccess, f.Reason)
	}

	failing := newID(kube.KindCronJob, "ns", "etl")
	put(m, failing, t0, map[string]inventory.Value{
		kube.AttrRunsSinceSuccess: inventory.Number(9),
		kube.AttrLastSuccess:      inventory.Time(t0.Add(-90 * 24 * time.Hour)),
	})
	failedRun(m, failing, "etl-1", t0, "BackoffLimitExceeded")
	for _, f := range evaluate(Schedule{}, m, t0, failing, nil).Findings {
		assert.NotEqual(t, reasons.CronJobNoRecentSuccess, f.Reason)
	}
}
