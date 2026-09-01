package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestContainerStateFilterTerminatedGraceful(t *testing.T) {
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

	filter := ContainerStateFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestContainerStateFilterTerminatedExitCode0(t *testing.T) {
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

	filter := ContainerStateFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestContainerStateFilterTerminatedCompletedWithRestarts(t *testing.T) {
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
	filter := ContainerStateFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestContainerStateFilterTerminatedGracefulWithRestarts(t *testing.T) {
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
	filter := ContainerStateFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestContainerStateFilterTerminatedCrashWithRestarts(t *testing.T) {
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

	filter := ContainerStateFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestContainerRestartsFilterNoState(t *testing.T) {
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

	filter := ContainerRestartsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.False(ctx.Container.HasRestarts)
}

func TestContainerRestartsFilterWithRestarts(t *testing.T) {
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

	filter := ContainerRestartsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.True(ctx.Container.HasRestarts)
}

func TestContainerRestartsFilterNoRestarts(t *testing.T) {
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

	filter := ContainerRestartsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.False(ctx.Container.HasRestarts)
}

func TestContainerKillingFilterDisabled(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{
				IgnoreFailedGracefulShutdown: false,
			},
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := ContainerKillingFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestContainerKillingFilterNilEvents(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{
				IgnoreFailedGracefulShutdown: true,
			},
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Events: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := ContainerKillingFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}
