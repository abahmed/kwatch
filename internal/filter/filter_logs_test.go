package filter

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestContainerReasonsFilterSameReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
		},
		Container: &ContainerContext{
			Reason:   "OOMKilled",
			Msg:      "killed",
			ExitCode: 137,
			Container: &corev1.ContainerStatus{
				Name: "test-container",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason: "OOMKilled",
					},
				},
			},
			LastState: &model.ContainerState{
				Reason:   "OOMKilled",
				Msg:      "killed",
				ExitCode: 137,
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := ContainerReasonsFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestContainerLogsFilterNoRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{
				MaxRecentLogLines: 10,
			},
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

	filter := ContainerLogsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestContainerLogsFilterCrashLoopBackOff(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Ctx:    context.Background(),
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{
				MaxRecentLogLines: 10,
			},
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

	filter := ContainerLogsFilter{}
	result := filter.Execute(ctx)
	// Should not short-circuit (return false means "don't stop processing"):
	// with RestartCount>0 and Waiting, previousLogs=true and it attempts log
	// fetch
	assert.False(result)
}

func TestContainerLogsFilterWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Ctx:    context.Background(),
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{
				MaxRecentLogLines: 10,
			},
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

	filter := ContainerLogsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestContainerLogsFilterIgnoredPattern(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{MaxRecentLogLines: 10}
	cfg.Suppression = config.SuppressionIndex{
		LogPatterns: []*regexp.Regexp{regexp.MustCompile("fake logs")},
	}
	ctx := &Context{
		Sources: Sources{
			Ctx:    context.Background(),
			Client: fake.NewSimpleClientset(),
			Config: cfg,
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

	filter := ContainerLogsFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}
