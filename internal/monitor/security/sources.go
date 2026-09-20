package security

import (
	admregv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
)

// Sources contains the informer-backed inputs for admission and webhook
// endpoint checks. The family receives one named bundle instead of positional
// listers that are easy to wire in the wrong order.
type Sources struct {
	MutatingWebhooks   admregv1lister.MutatingWebhookConfigurationLister
	ValidatingWebhooks admregv1lister.ValidatingWebhookConfigurationLister
	Services           corev1lister.ServiceLister
	EndpointSlices     discoveryv1lister.EndpointSliceLister
}

// SourceConfig is the one-time source wiring seam for the security family.
type SourceConfig interface {
	ConfigureSources(Sources) error
}
