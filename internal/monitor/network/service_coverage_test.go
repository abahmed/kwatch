package network

import (
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/constant"
)

func boolPtr(value bool) *bool { return &value }

func TestDetectServiceEndpointIssueHonorsEndpointReadiness(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}, Spec: corev1.ServiceSpec{
		Selector:  map[string]string{"app": "api"},
		ClusterIP: "10.0.0.1",
	}}
	ready := &discoveryv1.EndpointSlice{Endpoints: []discoveryv1.Endpoint{{
		Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
	}}}
	if got := DetectServiceEndpointIssue(
		service, []*discoveryv1.EndpointSlice{ready},
	); got != nil {
		t.Fatalf("ready endpoint produced signal: %+v", got)
	}
	if got := DetectServiceEndpointIssue(service, nil); got == nil ||
		got.Reason != constant.ReasonServiceNoEndpoints {
		t.Fatalf("missing endpoint signal = %+v", got)
	}
	service.Spec.ClusterIP = "None"
	if got := DetectServiceEndpointIssue(service, nil); got != nil {
		t.Fatal("headless service produced an endpoint signal")
	}
}

func TestDetectServicePortIssueFindsMissingNamedAndNumericPorts(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
		{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
	}}}
	if got := DetectServicePortIssue(service, nil); got != nil {
		t.Fatal("empty EndpointSlices should not be checked")
	}
	port := int32(8080)
	protocol := corev1.ProtocolTCP
	slice := &discoveryv1.EndpointSlice{Ports: []discoveryv1.EndpointPort{{
		Name: func() *string { value := "other"; return &value }(),
		Port: &port, Protocol: &protocol,
	}}}
	got := DetectServicePortIssue(
		service, []*discoveryv1.EndpointSlice{slice},
	)
	if got == nil || got.Reason != constant.ReasonServicePortMismatch {
		t.Fatalf("missing named port signal = %+v", got)
	}
}

func TestDetectServiceStatusIssueUsesConditionsAndSustainWindow(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
		CreationTimestamp: metav1.Time{Time: now.Add(-2 * time.Minute)},
	}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		Status: corev1.ServiceStatus{Conditions: []metav1.Condition{{
			Type: "Ready", Status: metav1.ConditionFalse,
			Reason: "Pending", Message: "provisioning",
		}}}}
	if got := DetectServiceStatusIssue(service, now, 60); got == nil ||
		got.Reason != constant.ReasonLoadBalancerPending {
		t.Fatalf("condition signal = %+v", got)
	}
	service.Status.Conditions = nil
	if got := DetectServiceStatusIssue(service, now, 180); got != nil {
		t.Fatal("sustain window was ignored")
	}
	if got := DetectServiceStatusIssue(service, now, 60); got == nil {
		t.Fatal("old load balancer without ingress was not reported")
	}
	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{
		IP: "10.0.0.2",
	}}
	if got := DetectServiceStatusIssue(service, now, 60); got != nil {
		t.Fatal("provisioned load balancer was reported")
	}
}

func TestIngressIssueReportsMissingBackendsAndErrors(t *testing.T) {
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}, Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					PathType: &pathType, Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{Name: "missing"},
					},
				}},
			}},
	}}}}
	findings, err := DetectIngressIssueWithLookup(
		ingress, func(string, string) (bool, error) {
			return false, nil
		})
	if err != nil || len(findings) != 1 {
		t.Fatalf("missing backend findings = %v, %v", findings, err)
	}
	_, err = DetectIngressIssueWithLookup(
		ingress, func(string, string) (bool, error) {
			return false, errors.New("cache unavailable")
		})
	if err == nil {
		t.Fatal("lookup error was swallowed")
	}
}

func TestEndpointCanReceiveTrafficFollowsServingAndTerminating(t *testing.T) {
	if !EndpointCanReceiveTraffic(discoveryv1.Endpoint{}) {
		t.Fatal("endpoint without conditions should be usable")
	}
	if !EndpointCanReceiveTraffic(discoveryv1.Endpoint{
		Conditions: discoveryv1.EndpointConditions{Serving: boolPtr(true)},
	}) {
		t.Fatal("serving endpoint should be usable")
	}
	if EndpointCanReceiveTraffic(discoveryv1.Endpoint{
		Conditions: discoveryv1.EndpointConditions{
			Serving: boolPtr(true), Terminating: boolPtr(true),
		},
	}) {
		t.Fatal("terminating endpoint should not be usable")
	}
}

func TestDetectNetworkPolicyIssueWithRules(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "deny-egress",
	}, Spec: networkingv1.NetworkPolicySpec{
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
	}}
	if got := DetectNetworkPolicyIssue(policy); got == nil ||
		got.Reason != constant.ReasonRestrictiveNetworkPolicy {
		t.Fatalf("deny-all policy signal = %+v", got)
	}
	policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
	if got := DetectNetworkPolicyIssue(policy); got != nil {
		t.Fatal("policy with egress rules was reported as deny-all")
	}
}
