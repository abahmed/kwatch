package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestPendingPodFilterNewPod(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				CreationTimestamp: metav1.NewTime(
					time.Now().Add(-1 * time.Minute),
				),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		},
	}

	filter := PendingPodFilter{Threshold: 5 * time.Minute}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.False(ctx.PodHasIssues)
}

func TestPendingPodFilterZeroCreationTimestampUsesNow(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
			Now:    func() time.Time { return now },
		},
		Pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}},
	}
	assert.False(t, (PendingPodFilter{Threshold: 5 * time.Minute}).Execute(ctx))
}

func TestPendingPodFilterOldPodNoWatchStart(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				CreationTimestamp: metav1.NewTime(
					time.Now().Add(-1 * time.Hour),
				),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: "Unschedulable",
					},
				},
			},
		},
	}

	filter := PendingPodFilter{Threshold: 5 * time.Minute}
	result := filter.Detect(ctx)
	assert.Equal(StatusAlert, result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("Unschedulable", ctx.PodReason)
}

func TestPendingPodFilterRestartGracePeriod(t *testing.T) {
	assert := assert.New(t)

	watchStart := time.Now().Add(-1 * time.Minute)
	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{
				WatchStartTime: watchStart,
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				CreationTimestamp: metav1.NewTime(
					time.Now().Add(-24 * time.Hour),
				),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		},
	}

	// With 5min threshold and only 1min since watch start, the filter should
	// skip
	filter := PendingPodFilter{Threshold: 5 * time.Minute}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.False(ctx.PodHasIssues)
}

func TestPendingPodFilterRestartAfterGracePeriod(t *testing.T) {
	assert := assert.New(t)

	watchStart := time.Now().Add(-10 * time.Minute)
	ctx := &Context{
		Sources: Sources{
			Config: &config.Config{
				WatchStartTime: watchStart,
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				CreationTimestamp: metav1.NewTime(
					time.Now().Add(-24 * time.Hour),
				),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: "Unschedulable",
					},
				},
			},
		},
	}

	// 10min since watch start, threshold is 5min → should alert
	filter := PendingPodFilter{Threshold: 5 * time.Minute}
	result := filter.Detect(ctx)
	assert.Equal(StatusAlert, result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("Unschedulable", ctx.PodReason)
}

func TestPodStatusFilterAlreadyKnown(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
		},
		Findings: Findings{
			PodHasIssues: true,
			PodLastState: &model.ContainerState{},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:    corev1.PodScheduled,
						Status:  corev1.ConditionFalse,
						Reason:  "Unschedulable",
						Message: "no nodes",
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestPodStatusFilterPodReadyToStartContainersFalse(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:    corev1.PodReadyToStartContainers,
						Status:  corev1.ConditionFalse,
						Reason:  "SandboxError",
						Message: "network plugin error",
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("SandboxError", ctx.PodReason)
	assert.Equal("network plugin error", ctx.PodMsg)
}

func TestContainerLogsFilterContainerStatusUnknown(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
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

	filter := ContainerLogsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Equal("", ctx.Container.Logs,
		"ContainerStatusUnknown should result in empty logs")
}

func TestPodOwnersFilterStatefulSet(t *testing.T) {
	assert := assert.New(t)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-sts",
			Namespace: "default",
		},
	}
	client := fake.NewSimpleClientset(sts)

	ctx := &Context{
		Sources: Sources{
			Client: client,
			Config: &config.Config{},
		},
		Owner: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-sts-0",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "my-sts",
						Kind: "StatefulSet",
					},
				},
			},
		},
	}

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.NotNil(ctx.Owner)
	assert.Equal("my-sts", ctx.Owner.Name)
	assert.Equal("StatefulSet", ctx.Owner.Kind)
}
