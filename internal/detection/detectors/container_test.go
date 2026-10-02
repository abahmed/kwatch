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

func TestContainerWaiting(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]inventory.Value{
			kube.AttrState: inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(
				reasons.ImagePullBackOff),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, container)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.ImagePullBackOff, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
}

func TestContainerCrashLoopBackOff(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]inventory.Value{
			kube.AttrState: inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(
				reasons.CrashLoopBackOff),
			kube.AttrLastReason: inventory.Text("Error"),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, container)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.CrashLoopBackOff, findings[0].Reason)
}

func TestContainerCrashLoopOOM(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]inventory.Value{
			kube.AttrState: inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(
				reasons.CrashLoopBackOff),
			kube.AttrLastReason: inventory.Text(
				reasons.OOMKilled),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, container)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.OOMKilled, findings[0].Reason)
	assert.Contains(t, findings[0].Summary, "memory limit")
}

func TestContainerTerminatedNormal(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]inventory.Value{
			kube.AttrState:    inventory.Text("terminated"),
			kube.AttrExitCode: inventory.Number(0),
			kube.AttrStateReason: inventory.Text(
				reasons.Completed),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, container)

	assert.Empty(t, findings)
}

func TestContainerTerminatedError(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]inventory.Value{
			kube.AttrState:       inventory.Text("terminated"),
			kube.AttrExitCode:    inventory.Number(1),
			kube.AttrStateReason: inventory.Text("Error"),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, container)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.Error, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
}

func TestContainerTerminatedOOM(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]inventory.Value{
			kube.AttrState:    inventory.Text("terminated"),
			kube.AttrExitCode: inventory.Number(137),
			kube.AttrStateReason: inventory.Text(
				reasons.OOMKilled),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, container)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.OOMKilled, findings[0].Reason)
}

func TestContainerInitError(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"init",
		since,
		map[string]inventory.Value{
			kube.AttrState:       inventory.Text("terminated"),
			kube.AttrExitCode:    inventory.Number(1),
			kube.AttrStateReason: inventory.Text("Error"),
			kube.AttrInit:        inventory.Bool(true),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, container)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.InitContainerError, findings[0].Reason)
}

func TestContainerRunningHighRestarts(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "default", Name: "pod-1/app"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrState:    inventory.Text("running"),
			kube.AttrRestarts: inventory.Number(8),
			kube.AttrLastFinished: inventory.Time(
				now.Add(-2 * time.Minute)),
		},
	})

	entity, _ := model.Entity(id)
	detector := Container{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.HighRestartCount, findings[0].Reason)
	assert.Contains(t, findings[0].Summary, "restarting")
}

func TestContainerRunningOOMRestarts(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "default", Name: "pod-1/app"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrState:    inventory.Text("running"),
			kube.AttrRestarts: inventory.Number(5),
			kube.AttrLastFinished: inventory.Time(
				now.Add(-2 * time.Minute)),
			kube.AttrLastReason: inventory.Text(
				reasons.OOMKilled),
		},
	})

	entity, _ := model.Entity(id)
	detector := Container{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.OOMKilled, findings[0].Reason)
	assert.Contains(t, findings[0].Summary, "memory limit")
}

// After kwatch restarts it sees an old restart count for the first time.
// The count is not recent just because kwatch only now observed it.
func TestContainerRestartCountSeenAfterKwatchRestart(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "pod-1"}
	cases := map[string]map[string]inventory.Value{
		"last restart long ago": {
			kube.AttrState:    inventory.Text("running"),
			kube.AttrRestarts: inventory.Number(9),
			kube.AttrLastReason: inventory.Text(
				reasons.OOMKilled),
			kube.AttrLastFinished: inventory.Time(
				now.Add(-3 * time.Hour)),
		},
		"no last termination time": {
			kube.AttrState:    inventory.Text("running"),
			kube.AttrRestarts: inventory.Number(9),
		},
	}
	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			// Observed "now": kwatch just started.
			c := buildContainer(pod, "app", now, attrs)
			ctx := testDetectorContext(nil, now)
			assert.Empty(t, Container{}.Detect(ctx, c))
		})
	}
}
