package security

import (
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
)

func TestDetectWebhookEndpointIssuesReportsUnavailableBackend(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	lister := discoveryv1lister.NewEndpointSliceLister(indexer)

	findings, err := DetectWebhookEndpointIssuesWithError(
		lister,
		"webhook",
		"default",
		map[string]string{"app": "admission"},
		[]*admissionregistrationv1.ServiceReference{{
			Namespace: "default",
			Name:      "admission",
		}},
	)
	if err != nil {
		t.Fatalf("detector returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
}

func TestDetectWebhookEndpointIssuesAcceptsReadyBackend(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	ready := true
	if err := indexer.Add(&discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admission-1",
			Namespace: "default",
			Labels: map[string]string{
				"kubernetes.io/service-name": "admission",
			},
		},
		Endpoints: []discoveryv1.Endpoint{{
			Conditions: discoveryv1.EndpointConditions{Ready: &ready},
		}},
	}); err != nil {
		t.Fatalf("add endpoint slice: %v", err)
	}

	findings, err := DetectWebhookEndpointIssuesWithError(
		discoveryv1lister.NewEndpointSliceLister(indexer),
		"webhook",
		"default",
		nil,
		[]*admissionregistrationv1.ServiceReference{{
			Namespace: "default",
			Name:      "admission",
		}},
	)
	if err != nil {
		t.Fatalf("detector returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %d, want 0", len(findings))
	}
}
