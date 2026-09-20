package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestContainerStateRuleTerminatedGraceful(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "Error",
						ExitCode: 143,
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerStateRuleTerminatedExitCode0(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "Test",
						ExitCode: 0,
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerStateRuleTerminatedCompletedWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			HasRestarts: true,
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "Completed",
						ExitCode: 0,
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

	// A cleanly-terminated container is not a failure even when the
	// restart count is non-zero (e.g. init container that failed once
	// then succeeded). It must be skipped, not alerted.
	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerStateRuleTerminatedGracefulWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			HasRestarts: true,
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "Error",
						ExitCode: 143,
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

	// Graceful SIGTERM shutdown must still be skipped with restarts,
	// while a genuine crash (non-zero, non-143 exit) still alerts.
	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerStateRuleTerminatedCrashWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			HasRestarts: true,
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "Error",
						ExitCode: 137,
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
}

func TestContainerStateRuleRunning(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
	assert.Equal("running", ctx.Container.Status)
}

func TestContainerStateRuleNilStatusIsSafe(t *testing.T) {
	ctx := &Context{Container: &ContainerContext{}}
	assert.Equal(t, DecisionAlert, (ContainerStateRule{}).Detect(ctx))
}

func TestContainerStateRuleRunningWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			HasRestarts: true,
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.Equal("running", ctx.Container.Status)
}

func TestContainerStateRuleWaiting(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.Equal("waiting", ctx.Container.Status)
}

func TestContainerStateRuleWaitingCreating(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "ContainerCreating",
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerStateRuleWaitingPodInitializing(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "PodInitializing",
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerStateRuleTerminated(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "OOMKilled",
						ExitCode: 137,
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.Equal("terminated", ctx.Container.Status)
}

func TestContainerStateRuleTerminatedCompleted(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "Completed",
						ExitCode: 0,
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

	rule := ContainerStateRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}
