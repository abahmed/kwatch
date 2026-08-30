package handler

import (
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
)

func mutatingWebhookServices(cfg *admissionregistrationv1.MutatingWebhookConfiguration) []*admissionregistrationv1.ServiceReference {
	refs := make([]*admissionregistrationv1.ServiceReference, 0, len(cfg.Webhooks))
	for i := range cfg.Webhooks {
		refs = append(refs, cfg.Webhooks[i].ClientConfig.Service)
	}
	return refs
}

func validatingWebhookServices(cfg *admissionregistrationv1.ValidatingWebhookConfiguration) []*admissionregistrationv1.ServiceReference {
	refs := make([]*admissionregistrationv1.ServiceReference, 0, len(cfg.Webhooks))
	for i := range cfg.Webhooks {
		refs = append(refs, cfg.Webhooks[i].ClientConfig.Service)
	}
	return refs
}

func (h *handler) detectWebhookEndpointIssues(name, namespace string, labelsMap map[string]string, refs []*admissionregistrationv1.ServiceReference) []*event.Signal {
	if h.listers.EndpointSlice == nil {
		return nil
	}
	var out []*event.Signal
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
		slices, err := h.listers.EndpointSlice.EndpointSlices(ref.Namespace).List(
			labels.Set{"kubernetes.io/service-name": ref.Name}.AsSelector(),
		)
		if err != nil {
			continue
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
			out = append(out, &event.Signal{
				Resource: "webhook", Namespace: namespace, PodName: name,
				Owner: name, Reason: constant.ReasonWebhookNoEndpoints,
				Labels: labelsMap,
				Hint:   fmt.Sprintf("webhook %s backend service %s has no usable endpoints", name, key),
			})
		}
	}
	return out
}
