package handler

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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
