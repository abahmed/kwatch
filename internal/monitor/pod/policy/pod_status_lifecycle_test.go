package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestPodStatusRuleAlreadyKnown(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestPodStatusRulePodReadyToStartContainersFalse(t *testing.T) {
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
						Type:    corev1.PodReadyToStartContainers,
						Status:  corev1.ConditionFalse,
						Reason:  "SandboxError",
						Message: "network plugin error",
					},
				},
			},
		},
	}

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("SandboxError", ctx.PodReason)
	assert.Equal("network plugin error", ctx.PodMsg)
}

func TestPodStatusRuleSucceeded(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}

func TestPodStatusRuleAddedWithNoConditions(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		EvType:  "Added",
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

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
}

func TestPodStatusRulePodCompleted(t *testing.T) {
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
						Reason: "PodCompleted",
					},
				},
			},
		},
	}

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
}

func TestPodStatusRulePodReady(t *testing.T) {
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
				},
			},
		},
	}

	rule := PodStatusRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}
