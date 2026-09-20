package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPendingPodRuleNewPod(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,
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

	rule := PendingPodRule{Threshold: 5 * time.Minute}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.PodHasIssues)
}

func TestPendingPodRuleZeroCreationTimestampUsesNow(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     func() time.Time { return now },
		Pod:     &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}},
	}
	assert.False(t, suppresses(PendingPodRule{Threshold: 5 * time.Minute}, ctx))
}

func TestPendingPodRuleOldPodNoWatchStart(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,
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

	rule := PendingPodRule{Threshold: 5 * time.Minute}
	result := rule.Detect(ctx)
	assert.Equal(DecisionAlert, result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("Unschedulable", ctx.PodReason)
}

func TestPendingPodRuleRestartGracePeriod(t *testing.T) {
	assert := assert.New(t)

	watchStart := time.Now().Add(-1 * time.Minute)
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			WatchStartTime: watchStart,
		}),
		Now: testNow,
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

	// With 5min threshold and only 1min since watch start, the rule should
	// skip
	rule := PendingPodRule{Threshold: 5 * time.Minute}
	result := suppresses(rule, ctx)
	assert.False(result)
	assert.False(ctx.PodHasIssues)
}

func TestPendingPodRuleRestartAfterGracePeriod(t *testing.T) {
	assert := assert.New(t)

	watchStart := time.Now().Add(-10 * time.Minute)
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			WatchStartTime: watchStart,
		}),
		Now: testNow,
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
	rule := PendingPodRule{Threshold: 5 * time.Minute}
	result := rule.Detect(ctx)
	assert.Equal(DecisionAlert, result)
	assert.True(ctx.PodHasIssues)
	assert.Equal("Unschedulable", ctx.PodReason)
}
