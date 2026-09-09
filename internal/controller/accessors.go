package controller

import (
	"sort"

	corev1lister "k8s.io/client-go/listers/core/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/metrics"
)

func (c *Controller) SetReadyFunc(fn func()) { c.readyFn = fn }

func (c *Controller) NamespaceAllowed(namespace string) bool {
	if _, forbidden := c.forbiddenNamespaces[namespace]; forbidden {
		return false
	}
	if c.watchAll || namespace == "" {
		return true
	}
	_, ok := c.allowedNamespaces[namespace]
	return ok
}

func makeNamespaceSet(namespaces []string) map[string]struct{} {
	set := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		set[namespace] = struct{}{}
	}
	return set
}

// NamespaceScope returns the namespaces selected by configuration. A true
// watchAll value means the selector is cluster-wide.
func (c *Controller) NamespaceScope() ([]string, bool) {
	if c.watchAll {
		return nil, true
	}
	namespaces := make([]string, 0, len(c.allowedNamespaces))
	for namespace := range c.allowedNamespaces {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces, false
}

func (c *Controller) SetTracker(t *kwcontext.ChangeTracker) { c.tracker = t }

// recordGraphSize publishes the graph's size so an empty graph — and therefore
// empty diagnoses — is visible on /metrics instead of only in the alerts that
// arrive without a cause.
func (c *Controller) recordGraphSize() {
	if c.graph == nil {
		return
	}
	nodes, edges := c.graph.Size()
	metrics.DefaultRegistry().GraphNodes.Store(int64(nodes))
	metrics.DefaultRegistry().GraphEdges.Store(int64(edges))
}
func (c *Controller) SetGraph(g *kwcontext.ResourceGraph) { c.graph = g }

// PodLister, ServiceLister and NodeLister expose the informer caches the
// controller already keeps synced.
//
// The out-of-band monitors -- kubelet telemetry, runtime metrics, active
// probes -- each listed the same objects from the API server on their own
// interval. Sharing the cache removes several cluster-wide LISTs a minute and
// removes the possibility of two components disagreeing about what exists.
// A nil return means that informer is not wired, and the caller keeps its own
// fallback.
func (c *Controller) PodLister() corev1lister.PodLister { return c.podLister }

func (c *Controller) ServiceLister() corev1lister.ServiceLister {
	return c.serviceLister
}

func (c *Controller) NodeLister() corev1lister.NodeLister {
	return c.nodeLister
}
