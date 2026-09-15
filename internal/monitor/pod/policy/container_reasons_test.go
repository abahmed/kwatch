package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestContainerReasonsRuleWaiting(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),

		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "ImagePullBackOff",
						Message: "image not found",
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

	rule := ContainerReasonsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.Equal("ImagePullBackOff", ctx.Container.Reason)
	assert.Equal("image not found", ctx.Container.Msg)
}

func TestContainerReasonsRuleTerminated(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),

		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:    "OOMKilled",
						Message:   "container killed",
						ExitCode:  137,
						StartedAt: metav1.Now(),
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

	rule := ContainerReasonsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.Equal("OOMKilled", ctx.Container.Reason)
	assert.Equal("container killed", ctx.Container.Msg)
	assert.Equal(int32(137), ctx.Container.ExitCode)
}

func TestContainerReasonsRuleCrashLoopBackOff(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),

		Container: &ContainerContext{
			HasRestarts: true,
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "CrashLoopBackOff",
					},
				},
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:    "Error",
						Message:   "exit with error",
						ExitCode:  1,
						StartedAt: metav1.Now(),
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

	rule := ContainerReasonsRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.Equal("Error", ctx.Container.Reason)
}

func TestContainerReasonsRuleAllowedReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			AllowedReasons: []string{"OOMKilled"},
		}),

		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason: "ImagePullBackOff",
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

	rule := ContainerReasonsRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerReasonsRuleForbiddenReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			ForbiddenReasons: []string{"ImagePullBackOff"},
		}),

		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff",
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

	rule := ContainerReasonsRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerReasonsRuleSameTerminatedTime(t *testing.T) {
	assert := assert.New(t)

	now := metav1.Now()

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),

		Container: &ContainerContext{
			LastTerminatedOn: now.Time,
			Container: &corev1.ContainerStatus{
				Name: "test-container",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:    "OOMKilled",
						StartedAt: now,
					},
				},
			},
			LastState: &model.ContainerState{
				LastTerminatedOn: now.Time,
				Reason:           "OOMKilled",
				Msg:              "killed",
				ExitCode:         137,
			},
		},

		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := ContainerReasonsRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}
