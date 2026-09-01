package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPodEventsFilterNilEvents(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
		},
		Findings: Findings{
			PodHasIssues: true,
		},
		Events: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := PodEventsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestPodEventsFilterWarningDeletingPod(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
		},
		Findings: Findings{
			PodHasIssues: true,
		},
		Events: &[]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Message: "deleting pod",
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := PodEventsFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}

func TestContainerNameFilterIgnored(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
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
	ctx.Config.Suppression = config.SuppressionIndex{
		ContainerNames: []string{"test-container"},
	}

	filter := ContainerNameFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestContainerNameFilterNoMatch(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
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
	ctx.Config.Suppression = config.SuppressionIndex{
		ContainerNames: []string{"skip-container"},
	}

	filter := ContainerNameFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestContainerNameFilterEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
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

	filter := ContainerNameFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestNoiseFilterEmptyReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{},
		},

		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	f := NoiseFilter{}
	result := f.Execute(ctx)
	assert.False(result)
}

func TestNoiseFilterNoiseReason(t *testing.T) {
	for _, reason := range noiseReasons {
		t.Run(reason, func(t *testing.T) {
			assert := assert.New(t)

			ctx := &Context{
				Container: &ContainerContext{
					Container: &corev1.ContainerStatus{},
					Reason:    reason,
				},

				Pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
					},
				},
			}

			f := NoiseFilter{}
			result := f.Execute(ctx)
			assert.True(result)
		})
	}
}

func TestNoiseFilterNonNoiseReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{},
			Reason:    "CrashLoopBackOff",
		},

		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	f := NoiseFilter{}
	result := f.Execute(ctx)
	assert.False(result)
}
