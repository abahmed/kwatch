package controller

import (
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/metrics"
)

func (c *Controller) SetReadyFunc(fn func()) { c.readyFn = fn }

func (c *Controller) NamespaceAllowed(namespace string) bool {
	if c.watchAll || namespace == "" {
		return true
	}
	_, ok := c.allowedNamespaces[namespace]
	return ok
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
	metrics.Default.GraphNodes.Store(int64(nodes))
	metrics.Default.GraphEdges.Store(int64(edges))
}
func (c *Controller) SetGraph(g *kwcontext.ResourceGraph) { c.graph = g }
