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

func TestPodRunningReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pod := buildPod("test", "default", now, map[string]knowledge.Value{
		kube.AttrPhase: knowledge.Text("Running"),
		kube.AttrReady: knowledge.Bool(true),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)
	ctx.Model = newTestModel()

	signals := detector.Detect(ctx, pod)
	assert.Empty(t, signals)
}

func TestPodPending(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	scheduled := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}

	condKey := kube.ConditionKey("PodScheduled")
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     scheduled,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase:                    knowledge.Text("Pending"),
			condKey:                           knowledge.Text("True"),
			condKey + kube.AttrConditionSince: knowledge.Time(scheduled),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPodPending, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
}

func TestPodUnschedulable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}

	condKey := kube.ConditionKey("PodScheduled")
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase:                     knowledge.Text("Pending"),
			condKey:                            knowledge.Text("False"),
			condKey + kube.AttrConditionReason: knowledge.Text("Unschedulable"),
			condKey + kube.AttrConditionMessage: knowledge.Text(
				"no nodes available"),
			condKey + kube.AttrConditionSince: knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonUnschedulable, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "Unschedulable")
}

func TestPodSchedulingGated(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-20 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}

	condKey := kube.ConditionKey("PodScheduled")
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase: knowledge.Text("Pending"),
			condKey:        knowledge.Text("False"),
			condKey + kube.AttrConditionReason: knowledge.Text(
				constant.ReasonSchedulingGated),
			condKey + kube.AttrConditionMessage: knowledge.Text(
				"waiting for gate"),
			condKey + kube.AttrConditionSince: knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonSchedulingGated, signals[0].Reason)
}

func TestPodFailed(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	failed := now.Add(-1 * time.Minute)

	pod := buildPod("test", "default", failed, map[string]knowledge.Value{
		kube.AttrPhase:   knowledge.Text("Failed"),
		kube.AttrReason:  knowledge.Text("Failed"),
		kube.AttrMessage: knowledge.Text("container exited"),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, pod)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPodFailed, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
}

func TestPodEvicted(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	failed := now.Add(-1 * time.Minute)

	pod := buildPod("test", "default", failed, map[string]knowledge.Value{
		kube.AttrPhase:   knowledge.Text("Failed"),
		kube.AttrReason:  knowledge.Text(constant.ReasonEvicted),
		kube.AttrMessage: knowledge.Text("Pod evicted"),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, pod)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonEvicted, signals[0].Reason)
}

func TestPodUnknownState(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	unknown := now.Add(-1 * time.Minute)

	pod := buildPod("test", "default", unknown, map[string]knowledge.Value{
		kube.AttrPhase: knowledge.Text("Unknown"),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, pod)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPodStatusUnknown, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
}

func TestPodStuckTerminating(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	deleting := now.Add(-15 * time.Minute)

	pod := buildPod("test", "default", deleting, map[string]knowledge.Value{
		kube.AttrPhase:    knowledge.Text("Running"),
		kube.AttrDeleting: knowledge.Bool(true),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, pod)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonPodStuckTerminating, signals[0].Reason)
}

func TestPodNotReadyShortDuration(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase:      knowledge.Text("Running"),
			kube.AttrReady:      knowledge.Bool(false),
			kube.AttrReadySince: knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	// Should not report signal yet - only 1 minute, threshold is 3 minutes
	assert.Empty(t, signals)
}

func TestPodNotReadyExceededThreshold(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase:      knowledge.Text("Running"),
			kube.AttrReady:      knowledge.Bool(false),
			kube.AttrReadySince: knowledge.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonContainersNotReady, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
}
