package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/model"
)

func TestContainerRestartsRuleNoState(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
			},
		},

		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := ContainerRestartsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.Container.HasRestarts)
}

func TestContainerRestartsRuleWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
			},
			LastState: &model.ContainerState{
				RestartCount: 1,
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := ContainerRestartsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.True(ctx.Container.HasRestarts)
}

func TestContainerRestartsRuleNoRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
			},
			LastState: &model.ContainerState{
				RestartCount: 5,
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := ContainerRestartsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.Container.HasRestarts)
}
