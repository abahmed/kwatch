package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPodStatusFilterContainersNotReady(t *testing.T) {
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
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{
				AllowedReasons: []string{"OOMKilled"},
			},
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

func TestPodStatusFilterForbiddenReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{
				ForbiddenReasons: []string{"Unschedulable"},
			},
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
