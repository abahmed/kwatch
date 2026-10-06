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

// replacementRig is a pod, started at t0, on a node created nodeAge
// before t0.
func replacementRig(
	nodeAge time.Duration,
) (*inventory.Model, inventory.EntityID) {
	model := newTestModel()
	node := newID(kube.KindNode, "", "n2")
	pod := newID(kube.KindPod, "shop", "web-1")
	put(model, node, t0, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(t0.Add(-nodeAge))})
	put(model, pod, t0, map[string]inventory.Value{
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(t0),
		kube.AttrCreated:    inventory.Time(t0)})
	relateEntity(model, pod, inventory.RunsOn, node)
	return model, pod
}

func podFindings(
	model *inventory.Model, pod inventory.EntityID, now time.Time,
) []string {
	ctx := testDetectorContext(model, now)
	return findingReasons(NewPod(PodThresholds{}).Detect(
		ctx, entityOf(model, pod)))
}

func TestPodOnFreshNodeGetsGraceBeforeNotReady(t *testing.T) {
	model, pod := replacementRig(2 * time.Minute)
	notReady := 3 * time.Minute

	assert.Empty(t, podFindings(model, pod, t0.Add(notReady+time.Minute)),
		"the usual threshold is not enough on a fresh node")
	assert.Empty(t, podFindings(model, pod,
		t0.Add(notReady+replacementGrace-time.Second)))
	got := podFindings(model, pod, t0.Add(notReady+replacementGrace))
	require.Equal(t, []string{reasons.ContainersNotReady}, got)
}

func TestPodOnOldNodeKeepsTheUsualThreshold(t *testing.T) {
	model, pod := replacementRig(time.Hour)

	got := podFindings(model, pod, t0.Add(4*time.Minute))

	assert.Equal(t, []string{reasons.ContainersNotReady}, got)
}

func TestReplacementGraceEndsWhenThePodIsNoLongerYoung(t *testing.T) {
	model, pod := replacementRig(2 * time.Minute)
	ctx := testDetectorContext(model, t0.Add(youngPod+time.Second))

	if got := replacementGraceFor(ctx, entityOf(model, pod)); got != 0 {
		t.Fatalf("an old pod gets no grace, got %v", got)
	}
}

func TestWorkloadAvailabilityGetsTheReplacementGrace(t *testing.T) {
	model, pod := replacementRig(2 * time.Minute)
	deploy := newID(kube.KindDeployment, "shop", "web")
	put(model, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(1),
		kube.AttrReadyReplicas:   inventory.Number(0),
		kube.AttrUpdatedReplicas: inventory.Number(1)})
	relateEntity(model, pod, inventory.OwnedBy, deploy)
	detect := func(after time.Duration) []string {
		ctx := testDetectorContext(model, t0.Add(after))
		return findingReasons(NewWorkload(5*time.Minute).Detect(
			ctx, entityOf(model, deploy)))
	}

	assert.Empty(t, detect(6*time.Minute), "grace holds it back")
	assert.Contains(t, detect(5*time.Minute+replacementGrace),
		reasons.DeploymentUnavailable)
}
