package security

import (
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// MutatingWebhookServices returns the Service backends named by a webhook
// configuration. Endpoint availability is a network concern, even though the
// configuration itself is an admission resource.
func MutatingWebhookServices(
	cfg *admissionregistrationv1.MutatingWebhookConfiguration,
) []*admissionregistrationv1.ServiceReference {
	refs := make([]*admissionregistrationv1.ServiceReference, 0,
		len(cfg.Webhooks))
	for i := range cfg.Webhooks {
		refs = append(refs, cfg.Webhooks[i].ClientConfig.Service)
	}
	return refs
}

// ValidatingWebhookServices returns the Service backends named by a webhook
// configuration.
func ValidatingWebhookServices(
	cfg *admissionregistrationv1.ValidatingWebhookConfiguration,
) []*admissionregistrationv1.ServiceReference {
	refs := make([]*admissionregistrationv1.ServiceReference, 0,
		len(cfg.Webhooks))
	for i := range cfg.Webhooks {
		refs = append(refs, cfg.Webhooks[i].ClientConfig.Service)
	}
	return refs
}

// DetectWebhookEndpointIssuesWithError reports webhook backends whose Service
// has no EndpointSlice endpoint able to receive traffic.
func DetectWebhookEndpointIssuesWithError(
	epLister discoveryv1lister.EndpointSliceLister,
	name, namespace string,
	labelsMap map[string]string,
	refs []*admissionregistrationv1.ServiceReference,
) ([]*model.Observation, error) {
	if epLister == nil {
		return nil, nil
	}
	var findings []*model.Observation
	seen := make(map[string]bool)
	for _, ref := range refs {
		if ref == nil || ref.Name == "" {
			continue
		}
		key := ref.Namespace + "/" + ref.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		slices, err := epLister.EndpointSlices(ref.Namespace).List(
			labels.Set{"kubernetes.io/service-name": ref.Name}.AsSelector(),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"list endpoint slices for webhook service %s: %w",
				key, err,
			)
		}
		if hasReadyEndpoint(slices) {
			continue
		}
		findings = append(findings, observe.ObjectNamed(
			"webhook", namespace, name,
			constant.ReasonWebhookNoEndpoints,
		).WithLabels(labelsMap).WithHint(fmt.Sprintf(
			"webhook %s backend service %s has no usable endpoints",
			name, key,
		)))
	}
	return findings, nil
}

func hasReadyEndpoint(slices []*discoveryv1.EndpointSlice) bool {
	for _, slice := range slices {
		for _, endpoint := range slice.Endpoints {
			if endpointCanReceiveTraffic(endpoint) {
				return true
			}
		}
	}
	return false
}

// endpointCanReceiveTraffic applies the EndpointSlice readiness semantics
// used by admission webhook backends.
func endpointCanReceiveTraffic(endpoint discoveryv1.Endpoint) bool {
	if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
		return false
	}
	if endpoint.Conditions.Serving != nil && !*endpoint.Conditions.Serving {
		return false
	}
	return endpoint.Conditions.Terminating == nil ||
		!*endpoint.Conditions.Terminating
}
