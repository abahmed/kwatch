package cluster

import (
	coordinationv1lister "k8s.io/client-go/listers/coordination/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

// Sources is the controller-owned cache view for cluster-resource
// processing. NamespaceAllowed is part of the same snapshot so source and
// scope configuration cannot be updated independently.
type Sources struct {
	ResourceQuotas   corev1lister.ResourceQuotaLister
	LimitRanges      corev1lister.LimitRangeLister
	Namespaces       corev1lister.NamespaceLister
	Leases           coordinationv1lister.LeaseLister
	NamespaceAllowed func(string) bool
}

// SourceConfig is the one controller-facing source wiring operation for the
// cluster-resource family.
type SourceConfig interface {
	ConfigureSources(Sources) error
}
