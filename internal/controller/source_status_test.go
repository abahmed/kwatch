package controller

import (
	"testing"
)

func TestUnavailableSourcesReportsOnlyActivePipelines(t *testing.T) {
	controller := &Controller{
		pipelineSet: pipelineSet{
			pod:  &resourcePipeline{startWorkers: true},
			node: &resourcePipeline{startWorkers: false},
		},
	}

	got := controller.unavailableSources()

	if len(got) != 3 || got[0] != "pod" || got[1] != "event" ||
		got[2] != "secret" {
		t.Fatalf("unavailableSources() = %v, want pod, event, secret", got)
	}
}
