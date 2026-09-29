package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestClaimBound(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	claim := buildStorage(
		kube.KindPVC, "data", "default", since,
		map[string]knowledge.Value{
			kube.AttrPhase: knowledge.Text("Bound"),
		},
	)

	detector := Claim{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, claim)

	assert.Empty(t, signals)
}

func TestClaimPending(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindPVC, Namespace: "default",
		Name: "data"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase: knowledge.Text("Pending"),
		},
	})

	entity, _ := model.Entity(id)
	detector := Claim{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPersistentVolumeClaim, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "waiting")
}

func TestClaimLost(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	claim := buildStorage(
		kube.KindPVC, "data", "default", since,
		map[string]knowledge.Value{
			kube.AttrPhase: knowledge.Text("Lost"),
		},
	)

	detector := Claim{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, claim)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPersistentVolumeClaim, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "lost")
}

func TestClaimResizeError(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindPVC, Namespace: "default",
		Name: "data"}

	condKey := kube.ConditionKey("ControllerResizeError")
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase:                     knowledge.Text("Bound"),
			condKey:                            knowledge.Text("True"),
			condKey + kube.AttrConditionReason: knowledge.Text("ResizeFailed"),
			condKey + kube.AttrConditionMessage: knowledge.Text(
				"Failed to resize"),
			condKey + kube.AttrConditionSince: knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := Claim{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPersistentVolumeClaim, signals[0].Reason)
	assert.Contains(t, signals[0].Summary, "resize failed")
}

func TestVolumeFailed(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	volume := buildStorage(
		kube.KindPV, "nfs-pv", "", since,
		map[string]knowledge.Value{
			kube.AttrPhase:   knowledge.Text("Failed"),
			kube.AttrReason:  knowledge.Text("VolumeFailed"),
			kube.AttrMessage: knowledge.Text("NFS mount error"),
		},
	)

	detector := Volume{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, volume)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPersistentVolume, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
}
