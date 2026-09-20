package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/metrics"
)

func (c *Controller) buildGraph() {
	if c.graph == nil {
		return
	}
	started := c.nowTime()
	metrics.DefaultRegistry().GraphRebuilds.Add(1)
	defer func() {
		metrics.DefaultRegistry().GraphRebuildLatencyMs.Store(
			c.nowTime().Sub(started).Milliseconds(),
		)
	}()

	next := c.newGraphBuilder(kwcontext.NewResourceGraph())
	if err := next.buildGraphContents(); err != nil {
		klog.ErrorS(err,
			"failed to rebuild dependency graph; keeping previous graph")
		return
	}
	c.graph.ReplaceWith(next.graph)
	klog.V(4).InfoS(
		"dependency graph built from informer cache",
		"edges", len(c.graph.Edges()),
	)
}

// newGraphBuilder snapshots only the informer inputs needed by a rebuild.
func (c *Controller) newGraphBuilder(
	graph *kwcontext.ResourceGraph,
) *graphBuilder {
	return &graphBuilder{
		graph: graph, podLister: c.podLister,
		nodeLister: c.nodeLister, pvcLister: c.pvcLister,
		pvLister: c.pvLister, rsLister: c.rsLister,
		jobLister: c.jobLister, serviceLister: c.serviceLister,
		ingressLister: c.ingressLister, hpaLister: c.hpaLister,
		netpolLister: c.netpolLister, pdbLister: c.pdbLister,
		endpointSliceLister: c.endpointSliceLister,
	}
}

func (b *graphBuilder) buildGraphContents() error {
	pods, err := b.podLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list pods for graph build: %w", err)
	}
	for _, pod := range pods {
		if err := b.addPodToGraphChecked(pod); err != nil {
			return fmt.Errorf(
				"build graph edges for pod %s/%s: %w",
				pod.Namespace, pod.Name, err,
			)
		}
	}
	return b.buildResourceGraph()
}

// removePodFromGraph removes all relationships owned by a deleted Pod.
func (c *Controller) removePodFromGraph(namespace, name string) {
	if c.graph != nil {
		c.graph.RemoveNode("pod", namespace, name)
	}
}
