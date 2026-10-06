package detectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cronWithRuns builds a CronJob whose finished Jobs took the given
// times, and one Job of it that started ran ago and still runs.
func cronWithRuns(
	took []time.Duration, ran time.Duration, extra map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "billing", "invoice-export")
	put(m, cron, t0.Add(-48*time.Hour), nil)
	for i, d := range took {
		done := newID(kube.KindJob, "billing", fmt.Sprintf("invoice-export-%d", i))
		started := t0.Add(-time.Duration(len(took)-i) * 24 * time.Hour)
		put(m, done, started, map[string]inventory.Value{
			kube.AttrStartTime:      inventory.Time(started),
			kube.AttrCompletionTime: inventory.Time(started.Add(d)),
		})
		setCondition(m, done, "Complete", "True", "", "", started.Add(d))
		link(m, done, inventory.OwnedBy, cron)
	}
	running := newID(kube.KindJob, "billing", "invoice-export-123")
	attrs := map[string]inventory.Value{
		kube.AttrStartTime: inventory.Time(t0.Add(-ran)),
		kube.AttrActive:    inventory.Number(1),
	}
	for name, value := range extra {
		attrs[name] = value
	}
	put(m, running, t0.Add(-ran), attrs)
	link(m, running, inventory.OwnedBy, cron)
	return m, running
}

var usualRuns = []time.Duration{8 * time.Minute, 12 * time.Minute,
	10 * time.Minute}

func TestJobReportsARunFarLongerThanTheRecentSuccesses(t *testing.T) {
	m, job := cronWithRuns(usualRuns, 2*time.Hour+10*time.Minute, nil)

	got := Job{}.Detect(testDetectorContext(m, t0), entityOf(m, job))

	require.Len(t, got, 1)
	assert.Equal(t, reasons.JobRunningLong, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, "2h10m", evidenceByLabel(got[0].Evidence)[detection.
		EvidenceRunningFor])
	assert.Equal(t, "8–12m", evidenceByLabel(got[0].Evidence)[detection.
		EvidenceRecentRuns])
}

func TestJobStaysQuietWithinThreeTimesTheLongestSuccess(t *testing.T) {
	// 36m is exactly 3 x 12m: not over it, and over the 30m floor.
	m, job := cronWithRuns(usualRuns, 35*time.Minute, nil)

	eval := evaluate(Job{}, m, t0, job, nil)

	assert.Empty(t, eval.Findings)
	assert.Equal(t, time.Minute, eval.RecheckAfter,
		"the Job is looked at again when it crosses the limit")
}

func TestJobStaysQuietBelowThirtyMinutesWhateverTheRatio(t *testing.T) {
	m, job := cronWithRuns([]time.Duration{time.Minute, time.Minute,
		2 * time.Minute}, 25*time.Minute, nil)

	got := Job{}.Detect(testDetectorContext(m, t0), entityOf(m, job))

	assert.Empty(t, got)
}

func TestJobNeedsThreeRecentSuccessesToJudgeARun(t *testing.T) {
	m, job := cronWithRuns(usualRuns[:2], 5*time.Hour, nil)

	got := Job{}.Detect(testDetectorContext(m, t0), entityOf(m, job))

	assert.Empty(t, got)
}

func TestJobLeavesARunItsDeadlineEndsSoonerToTheDeadline(t *testing.T) {
	m, job := cronWithRuns(usualRuns, 2*time.Hour, map[string]inventory.Value{
		kube.AttrActiveDeadline: inventory.Number(1800),
	})

	got := Job{}.Detect(testDetectorContext(m, t0), entityOf(m, job))

	assert.Empty(t, got)
}

func TestJobStillReportsWhenItsDeadlineIsFarOff(t *testing.T) {
	m, job := cronWithRuns(usualRuns, 2*time.Hour, map[string]inventory.Value{
		kube.AttrActiveDeadline: inventory.Number(24 * 3600),
	})

	got := Job{}.Detect(testDetectorContext(m, t0), entityOf(m, job))

	require.Len(t, got, 1)
	assert.Equal(t, reasons.JobRunningLong, got[0].Reason)
}

func TestJobIgnoresARunThatAlreadyCompleted(t *testing.T) {
	m, job := cronWithRuns(usualRuns, 2*time.Hour, nil)
	setCondition(m, job, "Complete", "True", "", "", t0)

	got := Job{}.Detect(testDetectorContext(m, t0), entityOf(m, job))

	assert.Empty(t, got)
}

func TestJobWithoutACronJobHasNoUsualRun(t *testing.T) {
	m := newTestModel()
	job := newID(kube.KindJob, "billing", "one-off")
	put(m, job, t0.Add(-5*time.Hour), map[string]inventory.Value{
		kube.AttrStartTime: inventory.Time(t0.Add(-5 * time.Hour)),
		kube.AttrActive:    inventory.Number(1),
	})

	got := Job{}.Detect(testDetectorContext(m, t0), entityOf(m, job))

	assert.Empty(t, got)
}

func TestRunRangeWritesWholeMinutesCompactly(t *testing.T) {
	assert.Equal(t, "8–12m", runRange([]time.Duration{
		8*time.Minute + 10*time.Second, 11*time.Minute + 40*time.Second}))
	assert.Equal(t, "45s–2h10m", runRange([]time.Duration{
		45 * time.Second, 2*time.Hour + 10*time.Minute}))
}
