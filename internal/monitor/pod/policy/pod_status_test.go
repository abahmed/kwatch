package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPodStatusRuleContainersNotReady(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),

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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.True(ctx.ContainersHasIssues)
}

func TestPodStatusRulePodNotScheduled(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),

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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("Unschedulable", ctx.PodReason)
}

func TestPodStatusRuleContainersReadyFalse(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),

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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.True(ctx.ContainersHasIssues)
}

func TestPodStatusRuleAllowedReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			AllowedReasons: []string{"OOMKilled"},
		}),

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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestPodStatusRuleForbiddenReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			ForbiddenReasons: []string{"Unschedulable"},
		}),

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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}
