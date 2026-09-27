package node

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestResolveNodeResourceCallsSinkWithNodeRef(t *testing.T) {
	sink := &nodeSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, nil, time.Now,
	)

	runtime.ResolveNodeResource("worker-1", "NodeResourceHigh")

	if sink.resolutions != 1 {
		t.Fatalf("expected 1 resolution, got %d", sink.resolutions)
	}
}

func TestResolveNodeResourceUsesNodeObjectRef(t *testing.T) {
	var gotRef model.ObjectRef
	var gotReason string
	sink := &resolveArgsRecorder{
		record: func(ref model.ObjectRef, reason string) {
			gotRef = ref
			gotReason = reason
		},
	}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, nil, time.Now,
	)

	runtime.ResolveNodeResource("worker-1", "NodeResourceHigh")

	want := model.ObjectRef{Kind: "node", Name: "worker-1"}
	if gotRef != want {
		t.Fatalf("expected ObjectRef %+v, got %+v", want, gotRef)
	}
	if gotReason != "NodeResourceHigh" {
		t.Fatalf("expected reason %q, got %q",
			"NodeResourceHigh", gotReason)
	}
}

type resolveArgsRecorder struct {
	record func(model.ObjectRef, string)
}

func (r *resolveArgsRecorder) Process(
	*model.Observation,
) (*model.Incident, model.IncidentAction) {
	return nil, model.ActionSkip
}

func (r *resolveArgsRecorder) Resolve(ref model.ObjectRef, reason string) {
	r.record(ref, reason)
}

func (r *resolveArgsRecorder) ResolveObserved(*model.Observation) {}
