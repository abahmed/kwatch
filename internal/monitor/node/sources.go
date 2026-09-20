package node

import corev1lister "k8s.io/client-go/listers/core/v1"

// Sources is the controller-owned cache view required by Node monitoring.
// Keeping it typed makes unavailable Node caches explicit to the runtime.
type Sources struct {
	Nodes corev1lister.NodeLister
}

// SourceConfig is the one source-wiring operation for the Node family.
type SourceConfig interface {
	ConfigureSources(Sources) error
}
