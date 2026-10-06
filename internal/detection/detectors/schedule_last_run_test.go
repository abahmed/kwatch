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

// failedRun is a Job of cron that failed at start with a reason.
func failedRun(m *inventory.Model, cron inventory.EntityID, name string,
	start time.Time, reason string,
) {
	id := newID(kube.KindJob, cron.Namespace, name)
	key := kube.ConditionKey("Failed")
	put(m, id, start, map[string]inventory.Value{
		kube.AttrStartTime:             inventory.Time(start),
		key:                            inventory.Text("True"),
		key + kube.AttrConditionReason: inventory.Text(reason),
		key + kube.AttrConditionSince:  inventory.Time(start),
	})
	link(m, id, inventory.OwnedBy, cron)
}

func TestScheduleReportsFailedLastRunForTheDigest(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "etl")
	next := t0.Add(3*24*time.Hour + 2*time.Hour)
	put(m, cron, t0, map[string]inventory.Value{
		kube.AttrNextRun: inventory.Time(next)})
	failedRun(m, cron, "etl-1", t0, "BackoffLimitExceeded")

	eval := evaluate(Schedule{}, m, t0.Add(3*24*time.Hour), cron, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, reasons.CronJobLastRunFailed, f.Reason)
	assert.Equal(t, detection.Info, f.Severity)
	assert.Equal(t, "Schedule.LastRunFailed", string(f.Mode))
	assert.Contains(t, f.Summary,
		"Last run failed 3 days ago (BackoffLimitExceeded); the next run "+
			"is at ")
}

func TestScheduleLastRunFailedClearsAfterSuccessOrDelete(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "etl")
	put(m, cron, t0, nil)
	failedRun(m, cron, "etl-1", t0, "DeadlineExceeded")
	now := t0.Add(2 * time.Hour)
	assert.Len(t, evaluate(Schedule{}, m, now, cron, nil).Findings, 1)

	finishedRun(m, cron, "etl-2", t0.Add(time.Hour), false)
	assert.Empty(t, evaluate(Schedule{}, m, now, cron, nil).Findings,
		"a later successful run clears it")

	m2 := newTestModel()
	put(m2, cron, t0, map[string]inventory.Value{
		kube.AttrLastSuccess: inventory.Time(t0.Add(time.Hour))})
	failedRun(m2, cron, "etl-1", t0, "DeadlineExceeded")
	assert.Empty(t, evaluate(Schedule{}, m2, now, cron, nil).Findings,
		"a success whose Job is gone clears it too")

	m3 := newTestModel()
	put(m3, cron, t0, nil)
	assert.Empty(t, evaluate(Schedule{}, m3, now, cron, nil).Findings,
		"no failed Job, no finding")
}

func TestScheduleLastRunFailedSkipsSuspendedCronJobs(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "etl")
	put(m, cron, t0, map[string]inventory.Value{
		kube.AttrSuspended: inventory.Bool(true)})
	failedRun(m, cron, "etl-1", t0, "BackoffLimitExceeded")

	eval := evaluate(Schedule{}, m, t0.Add(time.Hour), cron, nil)

	assert.Equal(t, []string{"CronJobSuspended"}, reasonsOf(eval.Findings))
}
