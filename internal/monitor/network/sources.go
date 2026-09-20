package network

import (
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
)

// Sources contains the informer-backed read-only inputs used by network
// queue processing. Named fields make family wiring reviewable at a glance.
type Sources struct {
	Services      corev1lister.ServiceLister
	EndpointSlice discoveryv1lister.EndpointSliceLister
	Ingresses     networkingv1lister.IngressLister
	NetworkPolicy networkingv1lister.NetworkPolicyLister
}

// SourceConfig is the one-time source wiring seam for the network family.
type SourceConfig interface {
	ConfigureSources(Sources) error
}
