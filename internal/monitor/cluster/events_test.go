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

func TestEventRuntimeIgnoresNonClusterWarnings(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sink := &eventSink{}
	runtime := NewEventRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.now = func() time.Time { return now }
	for _, event := range []*corev1.Event{
		nil,
		{Type: corev1.EventTypeNormal, Reason: "FailedMount"},
		{
			Type: corev1.EventTypeWarning, Reason: "FailedMount",
			InvolvedObject: corev1.ObjectReference{Kind: "Pod"},
		},
		{
			Type: corev1.EventTypeWarning, Reason: "FailedMount",
			Source: corev1.EventSource{Component: "cluster-autoscaler"},
		},
		{
			Type: corev1.EventTypeWarning, Reason: "Unrelated",
		},
	} {
		runtime.ProcessWarningEvent(event)
	}
	if len(sink.observations) != 0 {
		t.Fatalf("got %d observations, want none", len(sink.observations))
	}
}

func TestEventTimeUsesNewestAvailableSource(t *testing.T) {
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name  string
		event corev1.Event
	}{
		{
			name: "event time",
			event: corev1.Event{
				EventTime: metav1.MicroTime{Time: want},
			},
		},
		{
			name: "series time",
			event: corev1.Event{
				Series: &corev1.EventSeries{
					LastObservedTime: metav1.MicroTime{Time: want},
				},
			},
		},
		{
			name: "last timestamp",
			event: corev1.Event{
				LastTimestamp: metav1.Time{Time: want},
			},
		},
		{
			name: "first timestamp",
			event: corev1.Event{
				FirstTimestamp: metav1.Time{Time: want},
			},
		},
		{
			name: "creation timestamp",
			event: corev1.Event{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.Time{
					Time: want,
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := eventTime(&test.event); !got.Equal(want) {
				t.Fatalf("event time = %v, want %v", got, want)
			}
		})
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
