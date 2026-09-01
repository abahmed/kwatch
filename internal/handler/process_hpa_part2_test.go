package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHpaAtMaxMaxReplicasOne(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 1},
	}
	assert.False(t, hpaAtMax(hpa))
}

func TestDetectHPAIssuesNilIsSafe(t *testing.T) {
	assert.Empty(t, DetectHPAIssues(nil))
}

func TestHpaAtMaxScalingLimitedOtherReason(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 10},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:   autoscalingv2.ScalingLimited,
					Status: corev1.ConditionTrue,
					Reason: "SomeOtherReason",
				},
			},
		},
	}
	assert.False(t, hpaAtMax(hpa))
}

func TestHpaAtMaxTooManyReplicas(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 10},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:   autoscalingv2.ScalingLimited,
					Status: corev1.ConditionTrue,
					Reason: "TooManyReplicas",
				},
			},
		},
	}
	assert.True(t, hpaAtMax(hpa))
}

func TestHpaAtMaxScalingLimitedFalse(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 10},
	}
	assert.False(t, hpaAtMax(hpa))
}

func TestDetectHPAIssuesScalingDisabled(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa1", Namespace: "ns1"},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:   autoscalingv2.ScalingActive,
					Status: corev1.ConditionFalse,
					Reason: "ScalingDisabled",
				},
			},
		},
	}
	sigs := DetectHPAIssues(hpa)
	assert.Empty(t, sigs, "ScalingDisabled should not produce signals")
}
