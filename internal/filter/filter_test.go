package filter

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNamespaceFilterAllowed(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AllowedNamespaces: []string{"default", "kube-system"},
	}

	ctx := &Context{
		Sources: Sources{
			Client: client,
			Config: cfg,
		},
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
		Sources: Sources{
			Client: client,
			Config: cfg,
		},
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
		Sources: Sources{
			Client: client,
			Config: cfg,
		},
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
		Sources: Sources{
			Client: client,
			Config: cfg,
		},
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
	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		PodNamePatterns: []*regexp.Regexp{
			regexp.MustCompile("^test-.*"),
		},
	}

	ctx := &Context{
		Sources: Sources{
			Client: client,
			Config: cfg,
		},
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
	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		PodNamePatterns: []*regexp.Regexp{
			regexp.MustCompile("^skip-.*"),
		},
	}

	ctx := &Context{
		Sources: Sources{
			Client: client,
			Config: cfg,
		},
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
		Sources: Sources{
			Client: client,
			Config: cfg,
		},
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

func TestContainerStateFilterNilStatusIsSafe(t *testing.T) {
	ctx := &Context{Container: &ContainerContext{}}
	assert.Equal(t, StatusAlert, (ContainerStateFilter{}).Detect(ctx))
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
