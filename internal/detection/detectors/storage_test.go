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

func TestClaimBound(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	claim := buildStorage(
		kube.KindPVC, "data", "default", since,
		map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Bound"),
		},
	)

	detector := Claim{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, claim)

	assert.Empty(t, findings)
}

func TestClaimPending(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPVC, Namespace: "default",
		Name: "data"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Pending"),
		},
	})

	entity, _ := model.Entity(id)
	detector := Claim{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PersistentVolumeClaim, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "waiting")
}

func TestClaimLost(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	claim := buildStorage(
		kube.KindPVC, "data", "default", since,
		map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Lost"),
		},
	)

	detector := Claim{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, claim)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PersistentVolumeClaim, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "lost")
}

func TestClaimResizeError(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPVC, Namespace: "default",
		Name: "data"}

	condKey := kube.ConditionKey("ControllerResizeError")
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase:                     inventory.Text("Bound"),
			condKey:                            inventory.Text("True"),
			condKey + kube.AttrConditionReason: inventory.Text("ResizeFailed"),
			condKey + kube.AttrConditionMessage: inventory.Text(
				"Failed to resize"),
			condKey + kube.AttrConditionSince: inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := Claim{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PersistentVolumeClaim, findings[0].Reason)
	assert.Contains(t, findings[0].Summary, "resize failed")
}

func TestVolumeFailed(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	volume := buildStorage(
		kube.KindPV, "nfs-pv", "", since,
		map[string]inventory.Value{
			kube.AttrPhase:   inventory.Text("Failed"),
			kube.AttrReason:  inventory.Text("VolumeFailed"),
			kube.AttrMessage: inventory.Text("NFS mount error"),
		},
	)

	detector := Volume{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, volume)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PersistentVolume, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
}

func TestClaimLostWithResizeErrorReportsOneCriticalFinding(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)
	condKey := kube.ConditionKey("NodeResizeError")
	claim := buildStorage(
		kube.KindPVC, "data", "default", since,
		map[string]inventory.Value{
			kube.AttrPhase:                    inventory.Text("Lost"),
			condKey:                           inventory.Text("True"),
			condKey + kube.AttrConditionSince: inventory.Time(since),
		},
	)

	findings := Claim{}.Detect(testDetectorContext(nil, now), claim)

	require.Len(t, findings, 1)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "lost")
}
