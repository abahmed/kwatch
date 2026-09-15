package enrichment

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestContainerLogsEnricherContainerStatusUnknown(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client:  fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 3,
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason: "ContainerStatusUnknown",
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

	enricher := ContainerLogsEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
	assert.Equal("", ctx.Container.Logs,
		"ContainerStatusUnknown should result in empty logs")
}

func TestContainerLogsEnricherNoRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{
				MaxRecentLogLines: 10,
			}),
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 0,
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

	enricher := ContainerLogsEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}

func TestContainerLogsEnricherCrashLoopBackOff(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Ctx:    context.Background(),
			Client: fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{
				MaxRecentLogLines: 10,
			}),
		},
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
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	enricher := ContainerLogsEnricher{}
	result := enricher.Enrich(ctx)
	// Should not short-circuit (return false means "don't stop processing"):
	// with RestartCount>0 and Waiting, previousLogs=true and it attempts log
	// fetch
	assert.False(result)
}

func TestContainerLogsEnricherWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Ctx:    context.Background(),
			Client: fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{
				MaxRecentLogLines: 10,
			}),
		},
		Container: &ContainerContext{
			HasRestarts: true,
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
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

	enricher := ContainerLogsEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}

func TestContainerLogsEnricherIgnoredPattern(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{MaxRecentLogLines: 10}
	cfg.Suppression = config.SuppressionIndex{
		LogPatterns: []*regexp.Regexp{regexp.MustCompile("fake logs")},
	}
	ctx := &Context{
		Sources: Sources{
			Ctx:     context.Background(),
			Client:  fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(cfg),
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 0,
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

	enricher := ContainerLogsEnricher{}
	result := enricher.Enrich(ctx)
	assert.True(result)
}
