package filter

import (
	"regexp"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/storage"
	"github.com/abahmed/kwatch/internal/storage/memory"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNamespaceFilterAllowed(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AllowedNamespaces: []string{"default", "kube-system"},
	}

	ctx := &Context{
		Client: client,
		Config: cfg,
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := NamespaceFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestNamespaceFilterForbidden(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ForbiddenNamespaces: []string{"kube-system"},
	}

	ctx := &Context{
		Client: client,
		Config: cfg,
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "kube-system",
			},
		},
	}

	filter := NamespaceFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestNamespaceFilterNotInAllowedList(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AllowedNamespaces: []string{"kube-system"},
	}

	ctx := &Context{
		Client: client,
		Config: cfg,
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := NamespaceFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestNamespaceFilterNoConfig(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	ctx := &Context{
		Client: client,
		Config: cfg,
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := NamespaceFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestPodNameFilter(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		IgnorePodNamePatterns: []*regexp.Regexp{
			regexp.MustCompile("^test-.*"),
		},
	}

	ctx := &Context{
		Client: client,
		Config: cfg,
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := PodNameFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestPodNameFilterNoMatch(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		IgnorePodNamePatterns: []*regexp.Regexp{
			regexp.MustCompile("^skip-.*"),
		},
	}

	ctx := &Context{
		Client: client,
		Config: cfg,
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := PodNameFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestPodNameFilterEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	ctx := &Context{
		Client: client,
		Config: cfg,
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := PodNameFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
}

func TestContainerStateFilterRunning(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			},
		},
		Memory: memory.NewMemory(),
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
	assert.Equal("running", ctx.Container.Status)
}

func TestContainerStateFilterRunningWithRestarts(t *testing.T) {
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
		Memory: memory.NewMemory(),
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
	assert.Equal("running", ctx.Container.Status)
}

func TestContainerStateFilterWaiting(t *testing.T) {
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
		Memory: memory.NewMemory(),
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
	assert.Equal("waiting", ctx.Container.Status)
}

func TestContainerStateFilterWaitingCreating(t *testing.T) {
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
		Memory: memory.NewMemory(),
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

func TestContainerStateFilterWaitingPodInitializing(t *testing.T) {
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
		Memory: memory.NewMemory(),
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

func TestContainerStateFilterTerminated(t *testing.T) {
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
		Memory: memory.NewMemory(),
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
	assert.Equal("terminated", ctx.Container.Status)
}

func TestContainerStateFilterTerminatedCompleted(t *testing.T) {
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
		Memory: memory.NewMemory(),
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
		Memory: memory.NewMemory(),
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
		Memory: memory.NewMemory(),
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

func TestContainerRestartsFilterNoState(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
			},
		},
		Memory: memory.NewMemory(),
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

	mem := memory.NewMemory()
	mem.AddPodContainer("default", "test-pod", "test-container", &storage.ContainerState{
		RestartCount: 1,
	})

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
			},
		},
		Memory: mem,
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

	mem := memory.NewMemory()
	mem.AddPodContainer("default", "test-pod", "test-container", &storage.ContainerState{
		RestartCount: 5,
	})

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name:         "test-container",
				RestartCount: 5,
			},
		},
		Memory: mem,
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
		Config: &config.Config{
			IgnoreFailedGracefulShutdown: false,
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Memory: memory.NewMemory(),
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
		Config: &config.Config{
			IgnoreFailedGracefulShutdown: true,
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Events: nil,
		Memory: memory.NewMemory(),
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

func TestContainerKillingFilterWaitingState(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config: &config.Config{
			IgnoreFailedGracefulShutdown: true,
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
		Memory: memory.NewMemory(),
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
		Config: &config.Config{
			IgnoreFailedGracefulShutdown: true,
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
		Memory: memory.NewMemory(),
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
		Config: &config.Config{
			IgnoreFailedGracefulShutdown: true,
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
		Memory: memory.NewMemory(),
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
		Config: &config.Config{},
		Events: &[]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Message: "deleting pod",
			},
		},
		PodHasIssues: false,
		Memory:       memory.NewMemory(),
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

func TestPodEventsFilterNilEvents(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config:       &config.Config{},
		Events:       nil,
		PodHasIssues: true,
		Memory:       memory.NewMemory(),
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
		Config:       &config.Config{},
		PodHasIssues: true,
		Events: &[]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Message: "deleting pod",
			},
		},
		Memory: memory.NewMemory(),
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
		Config: &config.Config{
			IgnoreContainerNames: []string{"test-container"},
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := ContainerNameFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
}

func TestContainerNameFilterNoMatch(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config: &config.Config{
			IgnoreContainerNames: []string{"skip-container"},
		},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Memory: memory.NewMemory(),
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

func TestContainerNameFilterEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config: &config.Config{},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Memory: memory.NewMemory(),
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

func TestContainerReasonsFilterWaiting(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config: &config.Config{},
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
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := ContainerReasonsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Equal("ImagePullBackOff", ctx.Container.Reason)
	assert.Equal("image not found", ctx.Container.Msg)
}

func TestContainerReasonsFilterTerminated(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config: &config.Config{},
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
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := ContainerReasonsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Equal("OOMKilled", ctx.Container.Reason)
	assert.Equal("container killed", ctx.Container.Msg)
	assert.Equal(int32(137), ctx.Container.ExitCode)
}

func TestContainerReasonsFilterCrashLoopBackOff(t *testing.T) {
	assert := assert.New(t)

	mem := memory.NewMemory()

	ctx := &Context{
		Config: &config.Config{},
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
		Memory: mem,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := ContainerReasonsFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Equal("Error", ctx.Container.Reason)
}

func TestContainerReasonsFilterAllowedReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config: &config.Config{
			AllowedReasons: []string{"OOMKilled"},
		},
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
		Memory: memory.NewMemory(),
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

func TestContainerReasonsFilterForbiddenReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Config: &config.Config{
			ForbiddenReasons: []string{"ImagePullBackOff"},
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
		Memory: memory.NewMemory(),
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

func TestContainerReasonsFilterSameTerminatedTime(t *testing.T) {
	assert := assert.New(t)

	now := metav1.Now()

	mem := memory.NewMemory()
	mem.AddPodContainer("default", "test-pod", "test-container", &storage.ContainerState{
		LastTerminatedOn: now.Time,
		Reason:           "OOMKilled",
		Msg:              "killed",
		ExitCode:         137,
	})

	ctx := &Context{
		Config: &config.Config{},
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:    "OOMKilled",
						StartedAt: now,
					},
				},
			},
		},
		Memory: mem,
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

func TestContainerReasonsFilterSameReason(t *testing.T) {
	assert := assert.New(t)

	mem := memory.NewMemory()
	mem.AddPodContainer("default", "test-pod", "test-container", &storage.ContainerState{
		Reason:   "OOMKilled",
		Msg:      "killed",
		ExitCode: 137,
	})

	ctx := &Context{
		Config: &config.Config{},
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
		},
		Memory: mem,
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
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{
			MaxRecentLogLines: 10,
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
		Memory: memory.NewMemory(),
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

func TestContainerLogsFilterWithRestarts(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{
			MaxRecentLogLines: 10,
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
		Memory: memory.NewMemory(),
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

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{
			MaxRecentLogLines:         10,
			IgnoreLogPatternsCompiled: []*regexp.Regexp{regexp.MustCompile("fake logs")},
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
		Memory: memory.NewMemory(),
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

func TestPodOwnersFilterAlreadySet(t *testing.T) {
	assert := assert.New(t)

	owner := metav1.OwnerReference{
		Name: "existing-owner",
		Kind: "Deployment",
	}

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Owner:  &owner,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Equal("existing-owner", ctx.Owner.Name)
}

func TestPodOwnersFilterNoOwnerReferences(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Owner:  nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-pod",
				Namespace:       "default",
				OwnerReferences: []metav1.OwnerReference{},
			},
		},
	}

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Nil(ctx.Owner)
}

func TestPodOwnersFilterDirectOwner(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Owner:  nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "direct-deployment",
						Kind: "Deployment",
					},
				},
			},
		},
	}

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.NotNil(ctx.Owner)
	assert.Equal("direct-deployment", ctx.Owner.Name)
}

func TestPodOwnersFilterReplicaSet(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Owner:  nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "my-rs",
						Kind: "ReplicaSet",
					},
				},
			},
		},
	}

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.NotNil(ctx.Owner)
}

func TestPodStatusFilterSucceeded(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}

func TestPodStatusFilterAddedWithNoConditions(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		EvType: "Added",
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
}

func TestPodStatusFilterPodCompleted(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
						Reason: "PodCompleted",
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
}

func TestPodStatusFilterPodReady(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}

func TestPodStatusFilterContainersNotReady(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.True(ctx.ContainersHasIssues)
}

func TestPodStatusFilterPodNotScheduled(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:    corev1.PodScheduled,
						Status:  corev1.ConditionFalse,
						Reason:  "Unschedulable",
						Message: "no nodes available",
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("Unschedulable", ctx.PodReason)
}

func TestPodStatusFilterContainersReadyFalse(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{},
		Memory: memory.NewMemory(),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.True(ctx.ContainersHasIssues)
}

func TestPodStatusFilterAllowedReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{
			AllowedReasons: []string{"OOMKilled"},
		},
		Memory: memory.NewMemory(),
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

func TestPodStatusFilterForbiddenReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Client: fake.NewSimpleClientset(),
		Config: &config.Config{
			ForbiddenReasons: []string{"Unschedulable"},
		},
		Memory: memory.NewMemory(),
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

func TestPodStatusFilterAlreadyKnown(t *testing.T) {
	assert := assert.New(t)

	mem := memory.NewMemory()
	mem.AddPodContainer("default", "test-pod", ".", &storage.ContainerState{})

	ctx := &Context{
		Client:       fake.NewSimpleClientset(),
		Config:       &config.Config{},
		Memory:       mem,
		PodHasIssues: true,
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
