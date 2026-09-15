package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNotReadyRuleMeasuresFirstStartFromContainerStart(t *testing.T) {
	now := time.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: now.Add(-90 * time.Second)},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
				LastTransitionTime: metav1.Time{
					Time: now.Add(-90 * time.Second),
				},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.Time{Time: now.Add(-20 * time.Second)},
					},
				},
			}},
		},
	}
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Pod:     pod,
		Now:     testNow,
	}
	rule := NotReadyRule{Threshold: time.Minute}

	assert.Equal(t, DecisionDefer, rule.Detect(ctx),
		"twenty seconds after the container started is not a minute unready")
}

func TestNotReadyRuleStillAlertsWhenContainerStartedLongAgo(t *testing.T) {
	now := time.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: now.Add(-5 * time.Minute)},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
				LastTransitionTime: metav1.Time{
					Time: now.Add(-5 * time.Minute),
				},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.Time{Time: now.Add(-4 * time.Minute)},
					},
				},
			}},
		},
	}
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Pod:     pod,
		Now:     testNow,
	}
	rule := NotReadyRule{Threshold: time.Minute}

	assert.Equal(t, DecisionAlert, rule.Detect(ctx))
	assert.Contains(t, ctx.PodMsg, "never become ready")
}
