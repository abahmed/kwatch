package workload

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestDetectDeploymentConditionsPreservesFactsLabelsAndReasons(
	t *testing.T,
) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api", Namespace: "apps",
			Labels: map[string]string{"team": "payments"},
		},
		Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionFalse,
					Reason: "MinimumReplicasUnavailable",
				},
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionUnknown,
					Reason: "ReplicaSetUpdated",
				},
				{
					Type:   appsv1.DeploymentReplicaFailure,
					Status: corev1.ConditionTrue,
					Reason: "FailedCreate",
				},
			},
		},
	}

	observations := DetectDeploymentConditions(deploy)
	if len(observations) != 3 {
		t.Fatalf("observations = %d, want 3", len(observations))
	}
	wantReasons := []string{
		constant.ReasonDeploymentAvailable,
		constant.ReasonDeploymentProgressing,
		constant.ReasonDeploymentReplicaFailure,
	}
	for i, observation := range observations {
		if observation.Reason != wantReasons[i] {
			t.Errorf("observation %d reason = %q, want %q",
				i, observation.Reason, wantReasons[i])
		}
		if observation.Facts.DesiredReplicas != 3 ||
			observation.Facts.ReadyReplicas != 1 {
			t.Errorf("observation %d facts = %+v", i, observation.Facts)
		}
		if observation.Labels["team"] != "payments" {
			t.Errorf("observation %d labels = %+v", i, observation.Labels)
		}
	}
}
