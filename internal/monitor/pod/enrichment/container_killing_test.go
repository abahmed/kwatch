package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestContainerKillingEnricherWaitingState(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{
				IgnoreFailedGracefulShutdown: true,
			}),
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

	enricher := ContainerKillingEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}

func TestContainerKillingEnricherWithKillingEvent(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{
				IgnoreFailedGracefulShutdown: true,
			}),
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

	enricher := ContainerKillingEnricher{}
	result := enricher.Enrich(ctx)
	assert.True(result)
}

func TestContainerKillingEnricherWithOtherEvent(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{
				IgnoreFailedGracefulShutdown: true,
			}),
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

	enricher := ContainerKillingEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}

func TestContainerKillingEnricherDisabled(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{
				IgnoreFailedGracefulShutdown: false,
			}),
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

	enricher := ContainerKillingEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}

func TestContainerKillingEnricherNilEvents(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{
				IgnoreFailedGracefulShutdown: true,
			}),
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

	enricher := ContainerKillingEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}
