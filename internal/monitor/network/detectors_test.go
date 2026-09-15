package network

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestDetectNetworkPolicyIssueFindsDenyAllEgress(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-egress"},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	if got := DetectNetworkPolicyIssue(policy); got == nil {
		t.Fatal("expected restrictive policy finding")
	}
}

func TestDetectServiceEndpointIssueFindsMissingReadyEndpoints(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "api"},
			ClusterIP: "10.0.0.1",
		},
	}

	got := DetectServiceEndpointIssue(svc, nil)
	if got == nil || got.Reason != constant.ReasonServiceNoEndpoints {
		t.Fatal("expected service endpoint finding")
	}
}

func TestDetectIngressIssueReportsMissingBackend(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "missing",
								},
							},
						}},
					},
				},
			}},
		},
	}

	if got := DetectIngressIssue(ing, func(string, string) bool {
		return false
	}); len(got) != 1 {
		t.Fatalf("expected one missing backend finding, got %d", len(got))
	}
}
