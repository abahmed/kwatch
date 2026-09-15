package controller

import (
	"testing"

	"github.com/abahmed/kwatch/internal/metrics"
)

func TestControllerQueueDepthAggregatesActivePipelines(t *testing.T) {
	controller := &Controller{
		pipelineSet: pipelineSet{
			pod:  newResourcePipeline("pod", "pods-test"),
			node: newResourcePipeline("node", "nodes-test"),
		},
	}
	controller.pod.queue.Add("default/one")
	controller.pod.queue.Add("default/two")
	controller.node.queue.Add("node-one")
	controller.pod.queueDepth = controller.queueDepth
	controller.pod.recordQueueDepth()
	defer controller.pod.shutdown()
	defer controller.node.shutdown()

	if got := controller.queueDepth(); got != 3 {
		t.Fatalf("queue depth = %d, want aggregate depth 3", got)
	}
	if got := metrics.DefaultRegistry().QueueDepth.Load(); got != 3 {
		t.Fatalf("queue metric = %d, want aggregate depth 3", got)
	}
}
