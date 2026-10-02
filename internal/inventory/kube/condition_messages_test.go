package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func messageOf(t *testing.T, desc kube.Description, cond string) string {
	t.Helper()
	return desc.Attributes[kube.ConditionKey(cond)+
		kube.AttrConditionMessage].AsText()
}

func TestNodeSchemaConditionMessages(t *testing.T) {
	tt := []struct {
		name    string
		cond    corev1.NodeCondition
		wantMsg string
	}{
		{
			name: "not_ready_keeps_message",
			cond: corev1.NodeCondition{
				Type: corev1.NodeReady, Status: corev1.ConditionFalse,
				Message: "PLEG is not healthy",
			},
			wantMsg: "PLEG is not healthy",
		},
		{
			name: "pressure_true_keeps_message",
			cond: corev1.NodeCondition{
				Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue,
				Message: "kubelet has insufficient memory",
			},
			wantMsg: "kubelet has insufficient memory",
		},
		{
			name: "healthy_ready_drops_message",
			cond: corev1.NodeCondition{
				Type: corev1.NodeReady, Status: corev1.ConditionTrue,
				Message: "kubelet is posting ready status",
			},
			wantMsg: "",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			n := node("n1")
			n.Status.Conditions = []corev1.NodeCondition{tc.cond}
			desc, ok := kube.NodeSchema{}.Describe(n)
			assert.True(t, ok)
			assert.Equal(t, tc.wantMsg,
				messageOf(t, desc, string(tc.cond.Type)))
		})
	}
}

func TestNodeSchemaConditionMessageRedactsCredentials(t *testing.T) {
	n := node("n1")
	n.Status.Conditions = []corev1.NodeCondition{{
		Type: corev1.NodeReady, Status: corev1.ConditionFalse,
		Message: "pull failed password=hunter2 ok",
	}}
	desc, _ := kube.NodeSchema{}.Describe(n)
	assert.NotContains(t, messageOf(t, desc, "Ready"), "hunter2")
}

func TestWorkloadSchemaConditionMessages(t *testing.T) {
	failure := appsv1.DeploymentCondition{
		Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue,
		Message: "exceeded quota: compute",
	}
	healthy := appsv1.DeploymentCondition{
		Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue,
		Message: "Deployment has minimum availability.",
	}
	d := deployment("d1")
	d.Status.Conditions = []appsv1.DeploymentCondition{failure, healthy}
	desc, ok := kube.DeploymentSchema().Describe(d)
	assert.True(t, ok)
	assert.Equal(t, "exceeded quota: compute",
		messageOf(t, desc, "ReplicaFailure"))
	assert.Empty(t, messageOf(t, desc, "Available"))

	rs := replicaSet("rs1")
	rs.Status.Conditions = []appsv1.ReplicaSetCondition{{
		Type: appsv1.ReplicaSetReplicaFailure, Status: corev1.ConditionTrue,
		Message: "webhook denied the request",
	}}
	desc, _ = kube.ReplicaSetSchema().Describe(rs)
	assert.Equal(t, "webhook denied the request",
		messageOf(t, desc, "ReplicaFailure"))

	ss := statefulSet("ss1")
	ss.Status.Conditions = []appsv1.StatefulSetCondition{{
		Type: "Progressing", Status: corev1.ConditionFalse,
		Message: "update stalled",
	}}
	desc, _ = kube.StatefulSetSchema().Describe(ss)
	assert.Equal(t, "update stalled", messageOf(t, desc, "Progressing"))
}

func TestPodResizeConditionMessagesAreKept(t *testing.T) {
	for _, name := range []string{
		"PodResizePending", "PodResizeInProgress",
	} {
		t.Run(name, func(t *testing.T) {
			p := pod("p1")
			p.Status.Conditions = []corev1.PodCondition{{
				Type: corev1.PodConditionType(name), Status: corev1.ConditionTrue,
				Reason: "Deferred", Message: "node lacks memory",
			}}
			desc, ok := kube.PodSchema{}.Describe(p)
			assert.True(t, ok)
			assert.Equal(t, "node lacks memory", messageOf(t, desc, name))
		})
	}
}
