package node

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestNodeRuntimeReportsSustainedNotReadyNode(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := &config.Config{}
	cfg.NodeMonitor.SustainedMinutes = 0
	sink := &nodeSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfigFor(cfg), sink, nil, time.Now,
	)
	runtime.now = func() time.Time { return now }
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionFalse,
		}}},
	}

	if err := runtime.ProcessNodeObject(node, false); err != nil {
		t.Fatalf("ProcessNodeObject() returned error: %v", err)
	}
	if len(sink.findings) != 1 ||
		sink.findings[0].Reason != constant.ReasonNodeNotReady {
		t.Fatalf("unexpected findings: %#v", sink.findings)
	}
}

func TestNodeRuntimeSkipsDeletedNodeWhenListerIsUnavailable(t *testing.T) {
	sink := &nodeSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, nil, time.Now,
	)

	if err := runtime.ProcessNode("worker-1", true); err != nil {
		t.Fatalf("ProcessNode() returned error: %v", err)
	}
	if sink.resolutions != 0 {
		t.Fatalf("unavailable Node lister resolved %d incidents",
			sink.resolutions)
	}
}

type nodeSinkRecorder struct {
	findings    []*model.Observation
	resolutions int
}

func (r *nodeSinkRecorder) Process(
	finding *model.Observation,
) (*model.Incident, model.IncidentAction) {
	r.findings = append(r.findings, finding)
	return nil, model.ActionSkip
}

func (r *nodeSinkRecorder) Resolve(model.ObjectRef, string) {
	r.resolutions++
}

func (r *nodeSinkRecorder) ResolveObserved(*model.Observation) {}
