package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestDetectDeploymentIssueProgressDeadlineExceeded(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "ns1"},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"},
			},
		},
	}
	sig := DetectDeploymentIssue(deploy)
	assert.NotNil(t, sig)
	assert.Equal(t, "ProgressDeadlineExceeded", sig.Reason)
}

func TestDetectDeploymentIssueNoIssue(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "ns1"},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
			},
		},
	}
	assert.Nil(t, DetectDeploymentIssue(deploy))
}

func TestDetectDeploymentUnavailable(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "ns1"},
		Status: appsv1.DeploymentStatus{
			Replicas:            3,
			UnavailableReplicas: 2,
			ObservedGeneration:  1,
		},
	}
	sig := DetectDeploymentUnavailable(deploy)
	assert.NotNil(t, sig)
	assert.Equal(t, "DeploymentUnavailable", sig.Reason)
	assert.Equal(t, "ns1/dep1", sig.Owner)
}

func TestDetectDeploymentUnavailableMidRollout(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "ns1", Generation: 2},
		Status: appsv1.DeploymentStatus{
			Replicas:            3,
			UnavailableReplicas: 2,
			ObservedGeneration:  1,
		},
	}
	assert.Nil(t, DetectDeploymentUnavailable(deploy), "must not alert mid-rollout (stale observed generation)")
}

func TestDetectDeploymentUnavailableNoIssue(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "ns1"},
		Status: appsv1.DeploymentStatus{
			Replicas:            3,
			UnavailableReplicas: 0,
			ObservedGeneration:  1,
		},
	}
	assert.Nil(t, DetectDeploymentUnavailable(deploy))
}

func TestAvailabilityHintDeploy(t *testing.T) {
	deploy := &appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			Replicas:            5,
			ReadyReplicas:       3,
			UpdatedReplicas:     2,
			UnavailableReplicas: 2,
		},
	}
	hint := availabilityHintDeploy(deploy)
	assert.Contains(t, hint, "2/5")
}

func TestProcessDeploymentObjectNil(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	assert.NoError(t, h.ProcessDeploymentObject(nil, false))
}
