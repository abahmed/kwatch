package handler

import (
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// MutatingWebhookServices lists the Service backends a configuration names.
func MutatingWebhookServices(
	cfg *admissionregistrationv1.MutatingWebhookConfiguration,
) []*admissionregistrationv1.ServiceReference {
	refs := make([]*admissionregistrationv1.ServiceReference, 0, len(cfg.Webhooks))
	for i := range cfg.Webhooks {
		refs = append(refs, cfg.Webhooks[i].ClientConfig.Service)
	}
	return refs
}

// ValidatingWebhookServices lists the Service backends a configuration names.
func ValidatingWebhookServices(
	cfg *admissionregistrationv1.ValidatingWebhookConfiguration,
) []*admissionregistrationv1.ServiceReference {
	refs := make([]*admissionregistrationv1.ServiceReference, 0, len(cfg.Webhooks))
	for i := range cfg.Webhooks {
		refs = append(refs, cfg.Webhooks[i].ClientConfig.Service)
	}
	return refs
}

func (h *handler) detectWebhookEndpointIssues(
	name, namespace string,
	labelsMap map[string]string,
	refs []*admissionregistrationv1.ServiceReference,
) ([]*model.Observation, error) {
	return DetectWebhookEndpointIssuesWithError(
		h.listers.EndpointSlice, name, namespace, labelsMap, refs,
	)
}

// DetectWebhookEndpointIssues reports webhooks whose backend Service has no
// endpoint able to receive traffic.
//
// Exported so the startup baseline can run exactly the detector the live path
// runs. A detector reachable from only one of the two is a detector whose
// findings are re-announced as new on every restart.
func DetectWebhookEndpointIssues(
	epLister discoveryv1lister.EndpointSliceLister,
	name, namespace string,
	labelsMap map[string]string,
	refs []*admissionregistrationv1.ServiceReference,
) []*model.Observation {
	findings, err := DetectWebhookEndpointIssuesWithError(
		epLister, name, namespace, labelsMap, refs,
	)
	if err != nil {
		return nil
	}
	return findings
}

// DetectWebhookEndpointIssuesWithError is the error-aware endpoint detector
// used by live processing and startup seeding. A cache List error means the
// endpoint state is unknown, not healthy.
func DetectWebhookEndpointIssuesWithError(
	epLister discoveryv1lister.EndpointSliceLister,
	name, namespace string,
	labelsMap map[string]string,
	refs []*admissionregistrationv1.ServiceReference,
) ([]*model.Observation, error) {
	if epLister == nil {
		return nil, nil
	}
	var out []*model.Observation
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
				key,
				err,
			)
		}
		ready := false
		for _, slice := range slices {
			for _, endpoint := range slice.Endpoints {
				if endpointCanReceiveTraffic(endpoint) {
					ready = true
					break
				}
			}
			if ready {
				break
			}
		}
		if !ready {
			out = append(out, observe.ObjectNamed(
				"webhook", namespace, name,
				constant.ReasonWebhookNoEndpoints,
			).WithLabels(labelsMap).WithHint(fmt.Sprintf(
				"webhook %s backend service %s has no usable endpoints",
				name, key,
			)))
		}
	}
	return out, nil
}
