package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func failedPod(reason, disruption string) inventory.Entity {
	attrs := map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	}
	if reason != "" {
		attrs[kube.AttrReason] = inventory.Text(reason)
	}
	if disruption != "" {
		key := kube.ConditionKey("DisruptionTarget")
		attrs[key] = inventory.Text("True")
		attrs[key+kube.AttrConditionReason] = inventory.Text(disruption)
	}
	return buildPod("p", "ns", time.Now(), attrs)
}

func TestPodDisruptionTargetIsNotAFailure(t *testing.T) {
	for _, disruption := range []string{
		"PreemptionByScheduler", "DeletionByTaintManager",
		"EvictionByEvictionAPI",
	} {
		reason := ""
		got := NewPod(PodThresholds{}).Detect(
			testDetectorContext(newTestModel(), time.Now()),
			failedPod(reason, disruption))
		assert.Empty(t, got, disruption)
	}
}

func TestPodNodePressureEvictionStaysVisible(t *testing.T) {
	for _, disruption := range []string{"", "TerminationByKubelet"} {
		got := NewPod(PodThresholds{}).Detect(
			testDetectorContext(newTestModel(), time.Now()),
			failedPod(reasons.Evicted, disruption))
		require.Len(t, got, 1, disruption)
		assert.Equal(t, reasons.Evicted, got[0].Reason)
	}
}

func TestPodFailedWithoutDisruptionIsReported(t *testing.T) {
	got := NewPod(PodThresholds{}).Detect(
		testDetectorContext(newTestModel(), time.Now()), failedPod("", ""))
	require.Len(t, got, 1)
	assert.Equal(t, reasons.PodFailed, got[0].Reason)
}

// A pod the kubelet ended at node shutdown is replaced when a
// controller owns it, and gone for good when none does.
func TestPodEndedByShutdownReportsOnlyWithoutOwner(t *testing.T) {
	pod := failedPod("Terminated", "TerminationByKubelet")
	bare := NewPod(PodThresholds{}).Detect(
		testDetectorContext(newTestModel(), time.Now()), pod)
	require.Len(t, bare, 1)
	assert.Equal(t, reasons.PodFailed, bare[0].Reason)

	model := newTestModel()
	owner := inventory.EntityID{Kind: kube.KindReplicaSet,
		Namespace: "ns", Name: "rs"}
	model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: time.Now(), Entity: owner})
	link(model, pod.ID, inventory.OwnedBy, owner)
	got := NewPod(PodThresholds{}).Detect(
		testDetectorContext(model, time.Now()), pod)
	assert.Empty(t, got)
}
