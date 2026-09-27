package network

import (
	"time"

	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
)

// Sources contains informer inputs and the controller-owned Service requeue
// callback needed to revisit sustain deadlines without another event.
type Sources struct {
	Services       corev1lister.ServiceLister
	Pods           corev1lister.PodLister
	Nodes          corev1lister.NodeLister
	EndpointSlice  discoveryv1lister.EndpointSliceLister
	Ingresses      networkingv1lister.IngressLister
	NetworkPolicy  networkingv1lister.NetworkPolicyLister
	RequeueService func(string, time.Duration)
}

// SourceConfig is the one-time source wiring seam for the network family.
type SourceConfig interface {
	ConfigureSources(Sources) error
}
