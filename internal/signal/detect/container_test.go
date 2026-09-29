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

func TestContainerWaiting(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	container := buildContainer(
		knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]knowledge.Value{
			kube.AttrState: knowledge.Text("waiting"),
			kube.AttrStateReason: knowledge.Text(
				constant.ReasonImagePullBackOff),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, container)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonImagePullBackOff, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
}

func TestContainerCrashLoopBackOff(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	container := buildContainer(
		knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]knowledge.Value{
			kube.AttrState: knowledge.Text("waiting"),
			kube.AttrStateReason: knowledge.Text(
				constant.ReasonCrashLoopBackOff),
			kube.AttrLastReason: knowledge.Text("Error"),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, container)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonCrashLoopBackOff, signals[0].Reason)
}

func TestContainerCrashLoopOOM(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	container := buildContainer(
		knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]knowledge.Value{
			kube.AttrState: knowledge.Text("waiting"),
			kube.AttrStateReason: knowledge.Text(
				constant.ReasonCrashLoopBackOff),
			kube.AttrLastReason: knowledge.Text(
				constant.ReasonOOMKilled),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, container)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonOOMKilled, signals[0].Reason)
	assert.Contains(t, signals[0].Summary, "memory limit")
}

func TestContainerTerminatedNormal(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]knowledge.Value{
			kube.AttrState:    knowledge.Text("terminated"),
			kube.AttrExitCode: knowledge.Number(0),
			kube.AttrStateReason: knowledge.Text(
				constant.ReasonCompleted),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, container)

	assert.Empty(t, signals)
}

func TestContainerTerminatedError(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]knowledge.Value{
			kube.AttrState:       knowledge.Text("terminated"),
			kube.AttrExitCode:    knowledge.Number(1),
			kube.AttrStateReason: knowledge.Text("Error"),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, container)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonError, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
}

func TestContainerTerminatedOOM(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app",
		since,
		map[string]knowledge.Value{
			kube.AttrState:    knowledge.Text("terminated"),
			kube.AttrExitCode: knowledge.Number(137),
			kube.AttrStateReason: knowledge.Text(
				constant.ReasonOOMKilled),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, container)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonOOMKilled, signals[0].Reason)
}

func TestContainerInitError(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	container := buildContainer(
		knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"init",
		since,
		map[string]knowledge.Value{
			kube.AttrState:       knowledge.Text("terminated"),
			kube.AttrExitCode:    knowledge.Number(1),
			kube.AttrStateReason: knowledge.Text("Error"),
			kube.AttrInit:        knowledge.Bool(true),
		},
	)

	detector := Container{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, container)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonInitContainerError, signals[0].Reason)
}

func TestContainerRunningHighRestarts(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindContainer,
		Namespace: "default", Name: "pod-1/app"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrState:    knowledge.Text("running"),
			kube.AttrRestarts: knowledge.Number(8),
		},
	})

	entity, _ := model.Entity(id)
	detector := Container{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonHighRestartCount, signals[0].Reason)
	assert.Contains(t, signals[0].Summary, "restarting")
}

func TestContainerRunningOOMRestarts(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindContainer,
		Namespace: "default", Name: "pod-1/app"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrState:    knowledge.Text("running"),
			kube.AttrRestarts: knowledge.Number(5),
			kube.AttrLastReason: knowledge.Text(
				constant.ReasonOOMKilled),
		},
	})

	entity, _ := model.Entity(id)
	detector := Container{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonOOMKilled, signals[0].Reason)
	assert.Contains(t, signals[0].Summary, "memory limit")
}
