package cluster

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

type eventSink struct {
	observations []*model.Observation
	resolutions  []string
}

func (s *eventSink) Process(
	observation *model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.observations = append(s.observations, observation)
	return nil, model.ActionSkip
}

func (s *eventSink) Resolve(subject model.ObjectRef, reason string) {
	s.resolutions = append(s.resolutions, subject.Kind+":"+reason)
}

func (s *eventSink) ResolveObserved(*model.Observation) {}

func TestEventRuntimeProcessesRecentWarning(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sink := &eventSink{}
	runtime := NewEventRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.now = func() time.Time { return now }
	runtime.ProcessWarningEvent(&corev1.Event{
		Type:   corev1.EventTypeWarning,
		Reason: "FailedMount",
		InvolvedObject: corev1.ObjectReference{
			Kind: "PersistentVolumeClaim", Namespace: "demo", Name: "data",
		},
		EventTime: metav1.MicroTime{Time: now.Add(-time.Minute)},
	})

	if len(sink.observations) != 1 {
		t.Fatalf("got %d observations, want 1", len(sink.observations))
	}
	if got := sink.observations[0].Reason; got != "FailedMount" {
		t.Fatalf("reason = %q, want FailedMount", got)
	}
}

func TestEventRuntimeIgnoresOldWarning(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sink := &eventSink{}
	runtime := NewEventRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.now = func() time.Time { return now }
	runtime.ProcessWarningEvent(&corev1.Event{
		Type:   corev1.EventTypeWarning,
		Reason: "FailedMount",
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Namespace: "demo", Name: "api",
		},
		EventTime: metav1.MicroTime{Time: now.Add(-genericEventMaxAge - time.Second)},
	})

	if len(sink.observations) != 0 {
		t.Fatalf("got %d observations, want none", len(sink.observations))
	}
}

func TestEventRuntimeSustainsAndResolvesAutoscalerFailure(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sink := &eventSink{}
	runtime := NewEventRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.now = func() time.Time { return now }
	failure := &corev1.Event{Reason: "FailedToScaleUp"}
	runtime.ProcessClusterAutoscalerEvent(failure)
	now = now.Add(caSustainedMinutes * time.Minute)
	runtime.ProcessClusterAutoscalerEvent(failure)
	runtime.ProcessClusterAutoscalerEvent(&corev1.Event{Reason: "ScaleDown"})

	if len(sink.observations) != 1 {
		t.Fatalf("got %d observations, want 1", len(sink.observations))
	}
	if len(sink.resolutions) != 2 {
		t.Fatalf("got %d resolutions, want 2", len(sink.resolutions))
	}
}
