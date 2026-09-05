package controller

import (
	"sort"

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
