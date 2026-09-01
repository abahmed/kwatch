package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestContainerKillingFilterWaitingState(t *testing.T) {
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
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff",
					},
				},
			},
		},
		Events: &[]corev1.Event{},
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

func TestContainerKillingFilterWithKillingEvent(t *testing.T) {
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
		Events: &[]corev1.Event{
			{
				Reason:  "Killing",
				Message: "Stopping container test-container",
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
	assert.True(result)
}

func TestContainerKillingFilterWithOtherEvent(t *testing.T) {
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
		Events: &[]corev1.Event{
			{
				Reason:  "Started",
				Message: "Started container test-container",
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

func TestPodEventsFilterNotPodHasIssues(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
		},
		Findings: Findings{
			PodHasIssues: false,
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
	assert.False(result)
}
