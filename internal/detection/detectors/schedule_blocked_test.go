package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var blockedStart = time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)

// blockedCron builds a nightly Forbid CronJob whose run from
// blockedStart is still active, and returns the CronJob and its pod.
func blockedCron(m *inventory.Model, waiting string) (
	cron, pod inventory.EntityID,
) {
	cron = newID(kube.KindCronJob, "ops", "nightly-report")
	put(m, cron, t0, map[string]inventory.Value{
		kube.AttrNextRun:           inventory.Time(blockedStart.AddDate(0, 0, 1)),
		kube.AttrLastSchedule:      inventory.Time(blockedStart),
		kube.AttrActive:            inventory.Number(1),
		kube.AttrConcurrencyPolicy: inventory.Text("Forbid"),
		kube.AttrSchedule:          inventory.Text("0 2 * * *"),
	})
	job := newID(kube.KindJob, "ops", "nightly-report-28812340")
	put(m, job, t0, map[string]inventory.Value{
		kube.AttrStartTime: inventory.Time(blockedStart),
		kube.AttrActive:    inventory.Number(1),
	})
	link(m, job, inventory.OwnedBy, cron)
	pod = newID(kube.KindPod, "ops", "nightly-report-28812340-x2k")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
	})
	link(m, pod, inventory.OwnedBy, job)
	box := newID(kube.KindContainer, "ops", "nightly-report-28812340-x2k/job")
	put(m, box, t0, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text(waiting),
	})
	link(m, box, inventory.PartOf, pod)
	return cron, pod
}

func TestBlockedCronJobNamesStuckJobAndPod(t *testing.T) {
	m := newTestModel()
	cron, _ := blockedCron(m, "ImagePullBackOff")
	now := time.Date(2024, 1, 6, 3, 0, 0, 0, time.UTC)

	eval := evaluate(Schedule{}, m, now, cron, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, "Schedule.Blocked", string(f.Mode))
	assert.Contains(t, f.Summary, "Skipped 5 scheduled runs")
	assert.Contains(t, f.Summary, "nightly-report-28812340")
	assert.Contains(t, f.Summary, "2024-01-01 02:00 UTC")
	assert.Contains(t, f.Summary, "pod stuck in ImagePullBackOff")
	values := evidenceValues(f)
	assert.Equal(t, "5", values["skipped runs"])
	assert.Equal(t, "nightly-report-28812340", values["blocking job"])
}

func TestBlockedCronJobWithoutPodStateStillNamesJob(t *testing.T) {
	m := newTestModel()
	cron, pod := blockedCron(m, "")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running"),
		kube.AttrReady: inventory.Bool(true),
	})
	now := time.Date(2024, 1, 6, 3, 0, 0, 0, time.UTC)

	eval := evaluate(Schedule{}, m, now, cron, nil)

	require.Len(t, eval.Findings, 1)
	assert.Contains(t, eval.Findings[0].Summary, "is still running")
	assert.NotContains(t, eval.Findings[0].Summary, "stuck in")
}
