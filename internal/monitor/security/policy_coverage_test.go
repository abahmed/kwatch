package security

import (
	"errors"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWebhookPoliciesCoverMissingAndHealthyServices(t *testing.T) {
	service := "backend"
	namespace := "apps"
	mutating := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "mutating"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			Name: "check.example",
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name: service, Namespace: namespace,
				},
			},
		}},
	}
	if got := DetectMutatingWebhookIssue(mutating, func(string, string) bool {
		return false
	}); len(got) != 1 {
		t.Fatalf("mutating findings = %d", len(got))
	}
	if got := DetectMutatingWebhookIssue(mutating, func(string, string) bool {
		return true
	}); len(got) != 0 {
		t.Fatalf("healthy mutating findings = %d", len(got))
	}
	validating := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "validating"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			Name: "check.example",
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name: service, Namespace: namespace,
				},
			},
		}},
	}
	if got := DetectValidatingWebhookIssue(validating, func(string, string) bool {
		return false
	}); len(got) != 1 {
		t.Fatalf("validating findings = %d", len(got))
	}
	if _, err := DetectMutatingWebhookIssueWithLookup(
		mutating, func(string, string) (bool, error) {
			return false, errors.New("lookup failed")
		},
	); err == nil {
		t.Fatal("lookup failure was not returned")
	}
	if DetectMutatingWebhookIssue(nil, nil) != nil ||
		DetectValidatingWebhookIssue(validating, nil) != nil {
		t.Fatal("nil webhook dependencies produced findings")
	}
}
