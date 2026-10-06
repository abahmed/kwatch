package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func scaleNote(
	m *inventory.Model, pod inventory.EntityID, reason, msg string,
	at time.Time,
) {
	m.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: at, Entity: pod,
		Note: inventory.Note{At: at, Source: "cluster-autoscaler",
			Reason: reason, Message: msg, Count: 1,
			Warning: reason == "FailedScaling"},
	})
}

func TestAutoscalerAddingNode(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	scaleNote(m, pod, kube.ReasonTriggeredScaleUp,
		"pod triggered scale-up: [{ng-1 1->2 (max: 5)}]",
		podNodeNow.Add(-3*time.Minute))
	ev := fitOf(t, m, pod)
	assert.Equal(t, []string{detection.AutoscalerAdding},
		fitLines(ev, detection.EvidenceAutoscaler))
	assert.Equal(t, []string{"pod triggered scale-up: [{ng-1 1->2 (max: 5)}]"},
		fitLines(ev, detection.EvidenceAutoscalerSays))
}

func TestAutoscalerCannotAddNode(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	scaleNote(m, pod, kube.ReasonNotTriggerScaleUp,
		"pod didn't trigger scale-up: 1 max node group size reached",
		podNodeNow.Add(-time.Minute))
	ev := fitOf(t, m, pod)
	assert.Equal(t, []string{detection.AutoscalerBlocked},
		fitLines(ev, detection.EvidenceAutoscaler))
}

func TestAutoscalerNewestEventWins(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	scaleNote(m, pod, kube.ReasonNotTriggerScaleUp, "no",
		podNodeNow.Add(-5*time.Minute))
	scaleNote(m, pod, kube.ReasonTriggeredScaleUp, "yes",
		podNodeNow.Add(-time.Minute))
	assert.Equal(t, []string{detection.AutoscalerAdding},
		fitLines(fitOf(t, m, pod), detection.EvidenceAutoscaler))
}

func TestAutoscalerScaleUpThatNeverArrivedIsLate(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	scaleNote(m, pod, kube.ReasonTriggeredScaleUp, "up",
		podNodeNow.Add(-30*time.Minute))
	assert.Equal(t, []string{detection.AutoscalerLate},
		fitLines(fitOf(t, m, pod), detection.EvidenceAutoscaler))
	entity, _ := m.Entity(pod)
	assert.Zero(t, scaleUpGrace(testDetectorContext(m, podNodeNow), entity))
}

// pendingTenMinutes makes the pod unschedulable since ten minutes ago.
func pendingTenMinutes(m *inventory.Model, pod inventory.EntityID) {
	since := podNodeNow.Add(-10 * time.Minute)
	cond := kube.ConditionKey("PodScheduled")
	observeEntity(m, pod, since, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
		cond:           inventory.Text("False"),
		cond + kube.AttrConditionReason: inventory.Text(
			"Unschedulable"),
		cond + kube.AttrConditionMessage: inventory.Text("0/1 nodes"),
		cond + kube.AttrConditionSince:   inventory.Time(since),
	})
}

func detectPod(m *inventory.Model, pod inventory.EntityID,
) []detection.Finding {
	entity, _ := m.Entity(pod)
	return NewPod(PodThresholds{}).Detect(
		testDetectorContext(m, podNodeNow), entity)
}

func TestAutoscalerAddingHoldsTheUnschedulableFinding(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	pendingTenMinutes(m, pod)
	assert.Len(t, detectPod(m, pod), 1)
	scaleNote(m, pod, kube.ReasonTriggeredScaleUp, "up",
		podNodeNow.Add(-time.Minute))
	assert.Empty(t, detectPod(m, pod))
}

func TestAutoscalerScaleUpThatNeverArrivedReleasesTheHold(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	pendingTenMinutes(m, pod)
	scaleNote(m, pod, kube.ReasonTriggeredScaleUp, "up",
		podNodeNow.Add(-20*time.Minute))
	assert.Len(t, detectPod(m, pod), 1)
}

func TestAutoscalerBlockedDoesNotHoldTheFinding(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	pendingTenMinutes(m, pod)
	scaleNote(m, pod, kube.ReasonNotTriggerScaleUp, "no",
		podNodeNow.Add(-time.Minute))
	found := detectPod(m, pod)
	assert.Len(t, found, 1)
	assert.Equal(t, []string{detection.AutoscalerBlocked},
		fitLines(found[0].Evidence, detection.EvidenceAutoscaler))
}

func TestAutoscalerNothingWithoutEvents(t *testing.T) {
	m := newTestModel()
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	assert.Empty(t, fitLines(fitOf(t, m, pod), detection.EvidenceAutoscaler))
}
