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

func findingModes(eval detection.Evaluation) []string {
	var out []string
	for _, f := range eval.Findings {
		out = append(out, string(f.Mode))
	}
	return out
}

func TestScheduleReportsMissedRunEvents(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "report")
	put(m, cron, t0, nil)
	warn(m, cron, t0, "TooManyMissedTimes",
		"too many missed start times")

	eval := evaluate(Schedule{}, m, t0.Add(time.Minute), cron, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.CronJobMissedRuns, eval.Findings[0].Reason)
	assert.Equal(t, "Schedule.Missed", string(eval.Findings[0].Mode))

	later := evaluate(Schedule{}, m, t0.Add(time.Hour), cron, nil)
	assert.Empty(t, later.Findings, "old events expire")
}

func TestScheduleReportsForbidBlockedRun(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "sync")
	attrs := map[string]inventory.Value{
		kube.AttrNextRun:           inventory.Time(t0),
		kube.AttrActive:            inventory.Number(1),
		kube.AttrConcurrencyPolicy: inventory.Text("Forbid"),
	}
	put(m, cron, t0, attrs)
	now := t0.Add(10 * time.Minute)

	eval := evaluate(Schedule{}, m, now, cron, nil)
	assert.Equal(t, []string{"Schedule.Blocked"}, findingModes(eval))

	attrs[kube.AttrConcurrencyPolicy] = inventory.Text("Allow")
	put(m, cron, t0, attrs)
	eval = evaluate(Schedule{}, m, now, cron, nil)
	assert.Equal(t, []string{"NotScheduling"}, findingModes(eval))
}

func finishedRun(m *inventory.Model, cron inventory.EntityID, name string,
	start time.Time, failed bool,
) {
	id := newID(kube.KindJob, cron.Namespace, name)
	conditionType := "Complete"
	if failed {
		conditionType = "Failed"
	}
	put(m, id, start, map[string]inventory.Value{
		kube.AttrStartTime:               inventory.Time(start),
		kube.ConditionKey(conditionType): inventory.Text("True"),
	})
	link(m, id, inventory.OwnedBy, cron)
}

func TestScheduleReportsRepeatedFailures(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "etl")
	put(m, cron, t0, nil)
	finishedRun(m, cron, "etl-1", t0, false)
	for i, name := range []string{"etl-2", "etl-3", "etl-4"} {
		finishedRun(m, cron, name, t0.Add(time.Duration(i+1)*time.Hour),
			true)
	}

	eval := evaluate(Schedule{}, m, t0.Add(5*time.Hour), cron, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Schedule.RepeatedFailure", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "last 3")
	assert.Contains(t, eval.Findings[0].Evidence, detection.Evidence{
		Label: "latest failed job", Value: "etl-4"})
}

func TestScheduleCountsRunsSinceSuccessWithShortHistory(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "etl")
	put(m, cron, t0, map[string]inventory.Value{
		kube.AttrRunsSinceSuccess: inventory.Number(4),
	})
	finishedRun(m, cron, "etl-9", t0, true)

	eval := evaluate(Schedule{}, m, t0.Add(time.Hour), cron, nil)

	assert.Equal(t, []string{"Schedule.RepeatedFailure"}, findingModes(eval))
}

func TestScheduleIgnoresRecoveredOrFewFailures(t *testing.T) {
	m := newTestModel()
	recovered := newID(kube.KindCronJob, "ns", "ok")
	put(m, recovered, t0, nil)
	finishedRun(m, recovered, "ok-1", t0, true)
	finishedRun(m, recovered, "ok-2", t0.Add(time.Hour), true)
	finishedRun(m, recovered, "ok-3", t0.Add(2*time.Hour), true)
	finishedRun(m, recovered, "ok-4", t0.Add(3*time.Hour), false)
	few := newID(kube.KindCronJob, "ns", "few")
	put(m, few, t0, nil)
	finishedRun(m, few, "few-1", t0, true)
	finishedRun(m, few, "few-2", t0.Add(time.Hour), true)
	now := t0.Add(4 * time.Hour)

	assert.Empty(t, evaluate(Schedule{}, m, now, recovered, nil).Findings)
	// Two failures are not repeated; the failed last run is reported.
	assert.Equal(t, []string{"Schedule.LastRunFailed"},
		findingModes(evaluate(Schedule{}, m, now, few, nil)))
}

// Slots the controller skipped (Forbid with a long run, a missed
// window) were never Jobs, so they are not failed runs.
func TestScheduleDoesNotCountSkippedSlotsAsFailures(t *testing.T) {
	m := newTestModel()
	forbid := newID(kube.KindCronJob, "ns", "forbid")
	put(m, forbid, t0, map[string]inventory.Value{
		kube.AttrRunsSinceSuccess:  inventory.Number(48),
		kube.AttrConcurrencyPolicy: inventory.Text("Forbid"),
	})
	finishedRun(m, forbid, "forbid-1", t0, true)
	missed := newID(kube.KindCronJob, "ns", "missed")
	put(m, missed, t0, map[string]inventory.Value{
		kube.AttrRunsSinceSuccess: inventory.Number(48),
	})
	finishedRun(m, missed, "missed-1", t0, true)
	warn(m, missed, t0, "MissSchedule", "Missed scheduled time to start")
	now := t0.Add(time.Minute)

	for _, cron := range []inventory.EntityID{forbid, missed} {
		for _, mode := range findingModes(evaluate(Schedule{}, m, now,
			cron, nil)) {
			assert.NotEqual(t, "Schedule.RepeatedFailure", mode, cron.Name)
		}
	}
}
