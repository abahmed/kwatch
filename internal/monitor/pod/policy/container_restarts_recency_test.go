package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func recentRestartsContext(
	restartCount int32, finishedAt time.Time, now time.Time,
) *Context {
	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: restartCount,
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						FinishedAt: metav1.NewTime(finishedAt),
					},
				},
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}
	if !now.IsZero() {
		ctx.Now = func() time.Time { return now }
	}
	return ctx
}

func TestContainerRestartsRuleRecentTerminationWithinWindow(t *testing.T) {
	assert := assert.New(t)

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	finishedAt := now.Add(-5 * time.Minute)
	ctx := recentRestartsContext(3, finishedAt, now)

	rule := ContainerRestartsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.True(ctx.Container.HasRestarts)
}

func TestContainerRestartsRuleTerminationOutsideWindow(t *testing.T) {
	assert := assert.New(t)

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	finishedAt := now.Add(-20 * time.Minute)
	ctx := recentRestartsContext(3, finishedAt, now)

	rule := ContainerRestartsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.Container.HasRestarts)
}

func TestContainerRestartsRuleZeroRestartCount(t *testing.T) {
	assert := assert.New(t)

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	finishedAt := now.Add(-1 * time.Minute)
	ctx := recentRestartsContext(0, finishedAt, now)

	rule := ContainerRestartsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.Container.HasRestarts)
}

func TestContainerRestartsRuleNoClock(t *testing.T) {
	assert := assert.New(t)

	finishedAt := time.Now().Add(-1 * time.Minute)
	ctx := recentRestartsContext(3, finishedAt, time.Time{})
	ctx.Now = nil

	rule := ContainerRestartsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.Container.HasRestarts)
}
