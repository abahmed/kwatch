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

// failedPodRun is a standalone Failed pod whose container ended
// finishedAgo before ageNow. kwatch saw the pod only at ageNow, as after
// its own restart, so the first sighting says nothing about the age.
var ageNow = time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

func failedPodRun(
	finishedAgo time.Duration, podReason string,
) (*inventory.Model, inventory.Entity, inventory.Entity) {
	model := newTestModel()
	pod := newID(kube.KindPod, "ns", "p")
	container := newID(kube.KindContainer, "ns", "p/c")
	attrs := map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	}
	if podReason != "" {
		attrs[kube.AttrReason] = inventory.Text(podReason)
	}
	put(model, pod, ageNow, attrs)
	put(model, container, ageNow, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("terminated"),
		kube.AttrExitCode:    inventory.Number(1),
		kube.AttrStateReason: inventory.Text(reasons.Error),
		kube.AttrFinished:    inventory.Time(ageNow.Add(-finishedAgo)),
	})
	link(model, container, inventory.PartOf, pod)
	p, _ := model.Entity(pod)
	c, _ := model.Entity(container)
	return model, p, c
}

func detectAged(
	model *inventory.Model, pod, container inventory.Entity,
) (podFound, containerFound detection.Evaluation) {
	registry := detection.NewRegistry(nil, NewPod(PodThresholds{}),
		Container{})
	return registry.Evaluate(model, ageNow, pod.ID),
		registry.Evaluate(model, ageNow, container.ID)
}

func TestFailedPodSinceIsWhenItsContainerFinished(t *testing.T) {
	model, pod, container := failedPodRun(5*time.Minute, "")

	podFound, containerFound := detectAged(model, pod, container)

	require.Len(t, podFound.Findings, 1)
	require.Len(t, containerFound.Findings, 1)
	finished := ageNow.Add(-5 * time.Minute)
	assert.True(t, finished.Equal(podFound.Findings[0].Since))
	assert.True(t, finished.Equal(containerFound.Findings[0].Since))
	assert.Equal(t, detection.Warning, podFound.Findings[0].Severity)
	assert.Equal(t, detection.Critical, containerFound.Findings[0].Severity)
	assert.Equal(t, EventWindow-5*time.Minute+time.Nanosecond,
		podFound.RecheckAfter)
}

// A failure older than EventWindow is no news: it goes to the digest,
// like an old eviction leaves no finding at all.
func TestFailedPodOlderThanEventWindowIsOnlyInformational(t *testing.T) {
	model, pod, container := failedPodRun(EventWindow+time.Minute, "")

	podFound, containerFound := detectAged(model, pod, container)

	require.Len(t, podFound.Findings, 1)
	require.Len(t, containerFound.Findings, 1)
	assert.Equal(t, detection.Info, podFound.Findings[0].Severity)
	assert.Equal(t, detection.Info, containerFound.Findings[0].Severity)
}

func TestEvictedPodAgeComesFromItsContainerNotFirstSight(t *testing.T) {
	model, pod, container := failedPodRun(EventWindow+time.Minute,
		reasons.Evicted)

	podFound, _ := detectAged(model, pod, container)

	assert.Empty(t, podFound.Findings)
}

func TestFailedPodWithoutContainersCountsFromItsPhase(t *testing.T) {
	model := newTestModel()
	id := newID(kube.KindPod, "ns", "p")
	put(model, id, ageNow, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	})

	got := detection.NewRegistry(nil, NewPod(PodThresholds{})).Evaluate(
		model, ageNow, id)

	require.Len(t, got.Findings, 1)
	assert.Equal(t, detection.Warning, got.Findings[0].Severity)
	assert.True(t, ageNow.Equal(got.Findings[0].Since))
}

// Any Job's failure, not only a CronJob run's, is informational after a
// day.
func TestFailedJobOlderThanADayIsOnlyInformational(t *testing.T) {
	m := newTestModel()
	job := newID(kube.KindJob, "ns", "migrate")
	key := kube.ConditionKey("Failed")
	failedAt := ageNow.Add(-30 * time.Hour)
	put(m, job, ageNow, map[string]inventory.Value{
		kube.AttrFailed:                inventory.Number(6),
		key:                            inventory.Text("True"),
		key + kube.AttrConditionReason: inventory.Text("BackoffLimitExceeded"),
		key + kube.AttrConditionSince:  inventory.Time(failedAt),
	})

	got := evaluate(Job{}, m, ageNow, job, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.JobBackoffLimitExceeded, got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
}

func TestRecentFailedJobIsStillAWarning(t *testing.T) {
	m := newTestModel()
	job := newID(kube.KindJob, "ns", "migrate")
	key := kube.ConditionKey("Failed")
	put(m, job, ageNow, map[string]inventory.Value{
		key:                           inventory.Text("True"),
		key + kube.AttrConditionSince: inventory.Time(ageNow.Add(-time.Hour)),
	})

	got := evaluate(Job{}, m, ageNow, job, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
}
