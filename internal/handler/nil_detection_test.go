package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
)

func TestDetectionEntryPointsHandleNilObjects(t *testing.T) {
	assert.Nil(t, DetectDeploymentIssue(nil))
	assert.Nil(t, DetectDeploymentUnavailable(nil))
	assert.Nil(t, DetectStatefulSetIssue(nil))
	assert.Nil(t, DetectDaemonSetIssue(nil))
	assert.Nil(t, DetectJobIssue(nil))
	assert.Nil(t, DetectCronJobIssue(nil, time.Now()))
	assert.Nil(t, DetectHPAIssues((*autoscalingv2.HorizontalPodAutoscaler)(nil)))
	assert.Nil(t, DetectPdbIssue((*policyv1.PodDisruptionBudget)(nil)))
	assert.Nil(t, DetectNetworkPolicyIssue((*networkingv1.NetworkPolicy)(nil)))
	assert.Empty(t, DetectIngressIssue(nil, func(string, string) bool { return false }))
	assert.Nil(t, DetectServiceEndpointIssue((*corev1.Service)(nil), []*discoveryv1.EndpointSlice{}))
	assert.Empty(t, DetectMutatingWebhookIssue(nil, func(string, string) bool { return false }))
	assert.Empty(t, DetectValidatingWebhookIssue((*admissionregistrationv1.ValidatingWebhookConfiguration)(nil), func(string, string) bool { return false }))
	assert.Nil(t, DetectControlPlanePodIssue(nil))
}
