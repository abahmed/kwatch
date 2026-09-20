package controller

import (
	"testing"
	"time"

	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/metrics"
)

func TestSourceUnavailableMetricCountsTransitions(t *testing.T) {
	controller := &Controller{
		pipelineSet: pipelineSet{
			node: newResourcePipeline("node", "nodes"),
		},
	}
	controller.node.startWorkers = true
	before := metrics.DefaultRegistry().SourceUnavailable.Load()
	controller.recordSourceUnavailableTransitions()
	controller.recordSourceUnavailableTransitions()
	got := metrics.DefaultRegistry().SourceUnavailable.Load() - before
	if got != 1 {
		t.Fatalf("unconfigured controller reported %d transitions", got)
	}
}

func TestInformerStatusRefreshesSourceTransitions(t *testing.T) {
	controller := &Controller{
		pipelineSet: pipelineSet{
			node: newResourcePipeline("node", "nodes"),
		},
		now: time.Now,
	}
	controller.node.startWorkers = true
	before := metrics.DefaultRegistry().SourceUnavailable.Load()

	controller.InformerStatus()
	controller.nodeLister = corev1lister.NewNodeLister(
		cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil),
	)
	controller.InformerStatus()
	controller.nodeLister = nil
	controller.InformerStatus()

	got := metrics.DefaultRegistry().SourceUnavailable.Load() - before
	if got != 2 {
		t.Fatalf("source transitions = %d, want loss and recovery loss", got)
	}
}
