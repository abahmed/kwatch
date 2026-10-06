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

func TestPodRunningReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pod := buildPod("test", "default", now, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running"),
		kube.AttrReady: inventory.Bool(true),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)
	ctx.Model = newTestModel()

	findings := detector.Detect(ctx, pod)
	assert.Empty(t, findings)
}

func TestPodPending(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	// Pending longer than three times the pending threshold.
	scheduled := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}

	condKey := kube.ConditionKey("PodScheduled")
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     scheduled,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase:                    inventory.Text("Pending"),
			condKey:                           inventory.Text("True"),
			condKey + kube.AttrConditionSince: inventory.Time(scheduled),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PodPending, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
}

// TestPodNotLookedAtByScheduler: a pending pod with no scheduling
// condition is waiting for a scheduler; it is reported from its creation
// once the pending threshold passes, never before.
func TestPodNotLookedAtByScheduler(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age  time.Duration
		want int
	}{{age: time.Minute, want: 0}, {age: 5 * time.Minute, want: 1}} {
		created := now.Add(-tc.age)
		pod := buildPod("test", "default", created,
			map[string]inventory.Value{
				kube.AttrPhase:   inventory.Text("Pending"),
				kube.AttrCreated: inventory.Time(created),
			})
		ctx := testDetectorContext(newTestModel(), now)
		findings := NewPod(PodThresholds{}).Detect(ctx, pod)
		require.Len(t, findings, tc.want, tc.age)
		if tc.want == 1 {
			assert.Equal(t, reasons.PodPending, findings[0].Reason)
			assert.True(t, created.Equal(findings[0].Since))
			assert.Contains(t, findings[0].Summary, "scheduler")
		}
	}
}

func TestPodUnschedulable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}

	condKey := kube.ConditionKey("PodScheduled")
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase:                     inventory.Text("Pending"),
			condKey:                            inventory.Text("False"),
			condKey + kube.AttrConditionReason: inventory.Text("Unschedulable"),
			condKey + kube.AttrConditionMessage: inventory.Text(
				"no nodes available"),
			condKey + kube.AttrConditionSince: inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.Unschedulable, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "Unschedulable")
}

func TestPodSchedulingGated(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-20 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}

	condKey := kube.ConditionKey("PodScheduled")
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Pending"),
			condKey:        inventory.Text("False"),
			condKey + kube.AttrConditionReason: inventory.Text(
				reasons.SchedulingGated),
			condKey + kube.AttrConditionMessage: inventory.Text(
				"waiting for gate"),
			condKey + kube.AttrConditionSince: inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.SchedulingGated, findings[0].Reason)
}

func TestPodFailed(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	failed := now.Add(-1 * time.Minute)

	pod := buildPod("test", "default", failed, map[string]inventory.Value{
		kube.AttrPhase:   inventory.Text("Failed"),
		kube.AttrReason:  inventory.Text("Failed"),
		kube.AttrMessage: inventory.Text("container exited"),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, pod)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PodFailed, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
}

func TestPodEvicted(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	failed := now.Add(-1 * time.Minute)

	pod := buildPod("test", "default", failed, map[string]inventory.Value{
		kube.AttrPhase:   inventory.Text("Failed"),
		kube.AttrReason:  inventory.Text(reasons.Evicted),
		kube.AttrMessage: inventory.Text("Pod evicted"),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, pod)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.Evicted, findings[0].Reason)
}

// An eviction happened once: after EventWindow the evicted pod, which
// stays in the API until garbage collection, is history, not a failure.
func TestPodEvictedStopsCountingAfterTheEventWindow(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	failed := now.Add(-EventWindow)

	pod := buildPod("test", "default", failed, map[string]inventory.Value{
		kube.AttrPhase:   inventory.Text("Failed"),
		kube.AttrReason:  inventory.Text(reasons.Evicted),
		kube.AttrMessage: inventory.Text("Pod evicted"),
	})
	detector := NewPod(PodThresholds{})

	assert.Empty(t, detector.Detect(testDetectorContext(nil, now), pod))
	young := testDetectorContext(nil, now.Add(-time.Second))
	assert.Len(t, detector.Detect(young, pod), 1)
}

func TestPodUnknownState(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	unknown := now.Add(-1 * time.Minute)

	pod := buildPod("test", "default", unknown, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Unknown"),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, pod)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PodStatusUnknown, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
}

func TestPodStuckTerminating(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	deleting := now.Add(-15 * time.Minute)

	pod := buildPod("test", "default", deleting, map[string]inventory.Value{
		kube.AttrPhase:    inventory.Text("Running"),
		kube.AttrDeleting: inventory.Bool(true),
	})

	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, pod)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PodStuckTerminating, findings[0].Reason)
}

func TestPodNotReadyShortDuration(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase:      inventory.Text("Running"),
			kube.AttrReady:      inventory.Bool(false),
			kube.AttrReadySince: inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	// Should not report finding yet - only 1 minute, threshold is 3 minutes
	assert.Empty(t, findings)
}

func TestPodNotReadyExceededThreshold(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "test"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase:      inventory.Text("Running"),
			kube.AttrReady:      inventory.Bool(false),
			kube.AttrReadySince: inventory.Time(since),
		},
	})

	entity, _ := model.Entity(id)
	detector := NewPod(PodThresholds{})
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.ContainersNotReady, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
}
