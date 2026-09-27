package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestServiceDegradationDistinguishesPartialFromOutage(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "api"},
			ClusterIP: "10.0.0.1",
		},
	}
	ready := &discoveryv1.EndpointSlice{
		Endpoints: []discoveryv1.Endpoint{{
			Conditions: discoveryv1.EndpointConditions{
				Ready: boolPtr(true),
			},
		}},
	}
	pods := []*corev1.Pod{
		backendPod("one", "node-a", false),
		backendPod("two", "node-b", true),
	}
	got := DetectServiceBackendDegradation(
		svc, []*discoveryv1.EndpointSlice{ready}, pods,
	)
	require.NotNil(t, got)
	assert.Equal(t, constant.ReasonServiceBackendsDegraded, got.Reason)
	assert.Equal(t, model.SeverityWarning, got.Severity)
	assert.Equal(t, 1, got.Facts.HealthyEndpoints)
	assert.Nil(t, DetectServiceEndpointIssue(
		svc, []*discoveryv1.EndpointSlice{ready},
	))
	assert.Nil(t, DetectServiceBackendDegradation(svc, nil, pods))
	pods[0].Status.Conditions[0].Status = corev1.ConditionTrue
	assert.Nil(t, DetectServiceBackendDegradation(
		svc, []*discoveryv1.EndpointSlice{ready}, pods,
	))
}
