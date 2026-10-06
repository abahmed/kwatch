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

// A zero hard limit means "none allowed": it is never a quota that ran out.
func TestQuotaSkipsZeroHardResources(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "ns", "compute")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted:     inventory.Text("limits.cpu,pods"),
		kube.AttrQuotaZeroHard: inventory.Text("pods"),
	})

	eval := evaluate(Quota{}, m, t0, id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Contains(t, eval.Findings[0].Summary, "limits.cpu")
	assert.NotContains(t, eval.Findings[0].Summary, "pods")
}

func TestQuotaWithOnlyZeroHardResourcesSaysNothing(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "ns", "none")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted:     inventory.Text("pods,services"),
		kube.AttrQuotaZeroHard: inventory.Text("pods,services"),
	})

	assert.Empty(t, evaluate(Quota{}, m, t0, id, nil).Findings)
}

// A zero limit that just refused a create is the reason pods are missing.
func TestQuotaWithZeroHardIsReportedOnceItRefusesACreate(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "ns", "zero-pods")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted:     inventory.Text("pods"),
		kube.AttrQuotaZeroHard: inventory.Text("pods"),
	})
	rs := newID(kube.KindReplicaSet, "ns", "blocked")
	put(m, rs, t0, nil)
	warn(m, rs, t0, "FailedCreate", `Error creating: pods "blocked-x" is `+
		`forbidden: exceeded quota: zero-pods, requested: pods=1, `+
		`used: pods=0, limited: pods=0`)

	got := evaluate(Quota{}, m, t0.Add(time.Minute), id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ResourceQuotaExhausted, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "allows none of: pods")
}

// A refusal that names another quota does not blame this zero limit.
func TestQuotaWithZeroHardIgnoresRefusalsByOtherQuotas(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "ns", "zero-pods")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted:     inventory.Text("pods"),
		kube.AttrQuotaZeroHard: inventory.Text("pods"),
	})
	rs := newID(kube.KindReplicaSet, "ns", "blocked")
	put(m, rs, t0, nil)
	warn(m, rs, t0, "FailedCreate", `Error creating: forbidden: `+
		`exceeded quota: compute, requested: cpu=1`)

	got := evaluate(Quota{}, m, t0.Add(time.Minute), id, nil).Findings

	assert.Empty(t, got)
}

func deletingPod(
	now time.Time, deadline time.Time, grace float64,
) inventory.Entity {
	return buildPod("p", "default", now.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrPhase:            inventory.Text("Running"),
			kube.AttrDeleting:         inventory.Bool(true),
			kube.AttrDeletionTime:     inventory.Time(deadline),
			kube.AttrTerminationGrace: inventory.Number(grace),
		})
}

// deletionTimestamp already holds the grace period: a pod is stuck only
// once that deadline has passed by the threshold.
func TestPodWithinItsGraceIsNotStuckTerminating(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pod := deletingPod(now, now.Add(45*time.Minute), 3600)

	got := NewPod(PodThresholds{}).Detect(testDetectorContext(nil, now), pod)

	assert.Empty(t, got, "the 1h grace period is still running")
}

func TestPodPastItsGraceIsStuckTerminatingSinceTheRequest(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(-11 * time.Minute)
	pod := deletingPod(now, deadline, 30)

	got := NewPod(PodThresholds{}).Detect(testDetectorContext(nil, now), pod)

	require.Len(t, got, 1)
	assert.Equal(t, reasons.PodStuckTerminating, got[0].Reason)
	assert.True(t, deadline.Add(-30*time.Second).Equal(got[0].Since))
}

// Failed runs from before a suspension are not part of the streak, and
// the slots of the suspension never became Jobs.
func TestScheduleStreakIgnoresRunsBeforeResume(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "etl")
	put(m, cron, t0, map[string]inventory.Value{
		kube.AttrSuspended: inventory.Bool(true)})
	finishedRun(m, cron, "etl-1", t0.Add(-3*time.Hour), true)
	finishedRun(m, cron, "etl-2", t0.Add(-2*time.Hour), true)
	finishedRun(m, cron, "etl-3", t0.Add(-time.Hour), true)
	put(m, cron, t0.Add(24*time.Hour), map[string]inventory.Value{
		kube.AttrSuspended:        inventory.Bool(false),
		kube.AttrRunsSinceSuccess: inventory.Number(30),
	})
	finishedRun(m, cron, "etl-4", t0.Add(25*time.Hour), true)

	eval := evaluate(Schedule{}, m, t0.Add(26*time.Hour), cron, nil)

	assert.NotContains(t, findingModes(eval), "Schedule.RepeatedFailure")
}
