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

func TestDeploymentAvailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Hour)

	deployment := buildWorkload(
		kube.KindDeployment, "app", "default", since,
		map[string]knowledge.Value{
			kube.AttrReplicas:        knowledge.Number(3),
			kube.AttrReadyReplicas:   knowledge.Number(3),
			kube.AttrUpdatedReplicas: knowledge.Number(3),
		},
	)

	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, deployment)

	assert.Empty(t, signals)
}

func TestDeploymentUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindDeployment, Namespace: "default",
		Name: "app"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrReplicas:        knowledge.Number(3),
			kube.AttrReadyReplicas:   knowledge.Number(1),
			kube.AttrUpdatedReplicas: knowledge.Number(3),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonDeploymentUnavailable, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
	assert.True(t, signals[0].Symptom)
	assert.Contains(t, signals[0].Summary, "1 of 3")
}

func TestDeploymentCompletelyUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindDeployment, Namespace: "default",
		Name: "app"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrReplicas:        knowledge.Number(3),
			kube.AttrReadyReplicas:   knowledge.Number(0),
			kube.AttrUpdatedReplicas: knowledge.Number(3),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, signal.Critical, signals[0].Severity)
}

func TestDeploymentProgressDeadline(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindDeployment, Namespace: "default",
		Name: "app"}

	condKey := kube.ConditionKey("Progressing")
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			condKey:                            knowledge.Text("False"),
			condKey + kube.AttrConditionReason: knowledge.Text(constant.ReasonProgressDeadlineExceeded),
			condKey + kube.AttrConditionMessage: knowledge.Text(
				"Deadline exceeded"),
			condKey + kube.AttrConditionSince: knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonProgressDeadlineExceeded, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
}

func TestStatefulSetUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindStatefulSet, Namespace: "default",
		Name: "db"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrReplicas:      knowledge.Number(3),
			kube.AttrReadyReplicas: knowledge.Number(1),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonStsUnavailable, signals[0].Reason)
}

func TestDaemonSetUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindDaemonSet, Namespace: "default",
		Name: "monitor"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrDesiredReplicas: knowledge.Number(3),
			kube.AttrReadyReplicas:   knowledge.Number(1),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonDaemonSetUnavailable, signals[0].Reason)
}

func TestJobFailed(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindJob, Namespace: "default",
		Name: "task"}

	condKey := kube.ConditionKey("Failed")
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrFailed:                     knowledge.Number(3),
			condKey:                             knowledge.Text("True"),
			condKey + kube.AttrConditionReason:  knowledge.Text("Failed"),
			condKey + kube.AttrConditionMessage: knowledge.Text("Pods failed"),
			condKey + kube.AttrConditionSince:   knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := Job{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonJobFailed, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
}

func TestJobBackoffLimitExceeded(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindJob, Namespace: "default",
		Name: "task"}

	condKey := kube.ConditionKey("Failed")
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrFailed:                    knowledge.Number(5),
			condKey:                            knowledge.Text("True"),
			condKey + kube.AttrConditionReason: knowledge.Text("BackoffLimitExceeded"),
			condKey + kube.AttrConditionMessage: knowledge.Text(
				"Backoff limit reached"),
			condKey + kube.AttrConditionSince: knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := Job{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonJobBackoffLimitExceeded, signals[0].Reason)
}

func TestHPAScalingFailure(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindHPA, Namespace: "default",
		Name: "app-scaler"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrCurrentReplicas: knowledge.Number(3),
			kube.AttrDesiredReplicas: knowledge.Number(3),
		},
	})

	setCondition(model, id, "ScalingActive", "False",
		"FailedGetResourceMetric", "Cannot get metrics", since)

	entity, _ := model.Entity(id)
	detector := HPA{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonFailedGetResourceMetric, signals[0].Reason)
}

func TestHPAMaxedOut(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindHPA, Namespace: "default",
		Name: "app-scaler"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrCurrentReplicas: knowledge.Number(10),
			kube.AttrDesiredReplicas: knowledge.Number(10),
			kube.AttrMaxReplicas:     knowledge.Number(10),
		},
	})

	setCondition(model, id, "ScalingLimited", "True", "TooManyReplicas",
		"At max replicas", since)

	entity, _ := model.Entity(id)
	detector := HPA{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonHPAMaxedOut, signals[0].Reason)
}
