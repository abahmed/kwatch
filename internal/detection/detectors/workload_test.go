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

func TestDeploymentAvailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Hour)

	deployment := buildWorkload(
		kube.KindDeployment, "app", "default", since,
		map[string]inventory.Value{
			kube.AttrReplicas:        inventory.Number(3),
			kube.AttrReadyReplicas:   inventory.Number(3),
			kube.AttrUpdatedReplicas: inventory.Number(3),
		},
	)

	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, deployment)

	assert.Empty(t, findings)
}

func TestDeploymentUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindDeployment, Namespace: "default",
		Name: "app"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrReplicas:        inventory.Number(3),
			kube.AttrReadyReplicas:   inventory.Number(1),
			kube.AttrUpdatedReplicas: inventory.Number(3),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.DeploymentUnavailable, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
	assert.True(t, findings[0].Symptom)
	assert.Contains(t, findings[0].Summary, "1 of 3")
}

func TestDeploymentCompletelyUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindDeployment, Namespace: "default",
		Name: "app"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrReplicas:        inventory.Number(3),
			kube.AttrReadyReplicas:   inventory.Number(0),
			kube.AttrUpdatedReplicas: inventory.Number(3),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, detection.Critical, findings[0].Severity)
}

func TestDeploymentProgressDeadline(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindDeployment, Namespace: "default",
		Name: "app"}

	condKey := kube.ConditionKey("Progressing")
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			condKey: inventory.Text("False"),
			condKey + kube.AttrConditionReason: inventory.Text(
				reasons.ProgressDeadlineExceeded),
			condKey + kube.AttrConditionMessage: inventory.Text(
				"Deadline exceeded"),
			condKey + kube.AttrConditionSince: inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.ProgressDeadlineExceeded, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
}

func TestStatefulSetUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindStatefulSet, Namespace: "default",
		Name: "db"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrReplicas:      inventory.Number(3),
			kube.AttrReadyReplicas: inventory.Number(1),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.StsUnavailable, findings[0].Reason)
}

func TestDaemonSetUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindDaemonSet, Namespace: "default",
		Name: "monitor"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrDesiredReplicas: inventory.Number(3),
			kube.AttrReadyReplicas:   inventory.Number(1),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewWorkload(5 * time.Minute)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.DaemonSetUnavailable, findings[0].Reason)
}

func TestJobFailed(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindJob, Namespace: "default",
		Name: "task"}

	condKey := kube.ConditionKey("Failed")
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrFailed:                     inventory.Number(3),
			condKey:                             inventory.Text("True"),
			condKey + kube.AttrConditionReason:  inventory.Text("Failed"),
			condKey + kube.AttrConditionMessage: inventory.Text("Pods failed"),
			condKey + kube.AttrConditionSince:   inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := Job{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.JobFailed, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
}

func TestJobBackoffLimitExceeded(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindJob, Namespace: "default",
		Name: "task"}

	condKey := kube.ConditionKey("Failed")
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrFailed:                    inventory.Number(5),
			condKey:                            inventory.Text("True"),
			condKey + kube.AttrConditionReason: inventory.Text("BackoffLimitExceeded"),
			condKey + kube.AttrConditionMessage: inventory.Text(
				"Backoff limit reached"),
			condKey + kube.AttrConditionSince: inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := Job{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.JobBackoffLimitExceeded, findings[0].Reason)
}

func TestHPAScalingFailure(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-HPAMetricsGrace)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindHPA, Namespace: "default",
		Name: "app-scaler"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrCurrentReplicas: inventory.Number(3),
			kube.AttrDesiredReplicas: inventory.Number(3),
		},
	})

	setCondition(model, id, "ScalingActive", "False",
		"FailedGetResourceMetric", "Cannot get metrics", since)

	entity, _ := model.Entity(id)
	detector := HPA{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.FailedGetResourceMetric, findings[0].Reason)
}

func TestHPAMaxedOut(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindHPA, Namespace: "default",
		Name: "app-scaler"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrCurrentReplicas: inventory.Number(10),
			kube.AttrDesiredReplicas: inventory.Number(10),
			kube.AttrMaxReplicas:     inventory.Number(10),
		},
	})

	setCondition(model, id, "ScalingLimited", "True", "TooManyReplicas",
		"At max replicas", since)

	entity, _ := model.Entity(id)
	detector := HPA{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.HPAMaxedOut, findings[0].Reason)
}
