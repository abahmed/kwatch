package controller

import (
	"testing"

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
