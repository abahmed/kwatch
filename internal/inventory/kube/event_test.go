package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func warningEvent(kind, field string) *corev1.Event {
	return &corev1.Event{
		Type: corev1.EventTypeWarning, Reason: "BackOff",
		Message: "restarting", Count: 3,
		InvolvedObject: corev1.ObjectReference{
			Kind: kind, Namespace: testNamespace, Name: "p",
			FieldPath: field,
		},
		Source: corev1.EventSource{Component: "kubelet"},
	}
}

func TestEventNoteIgnoresNormalAndForeignObjects(t *testing.T) {
	now := fixedTime()
	ev := warningEvent("Pod", "")
	ev.Type = corev1.EventTypeNormal
	_, ok := kube.EventNote(ev, now)
	assert.False(t, ok)
	_, ok = kube.EventNote("nope", now)
	assert.False(t, ok)
}

func TestEventNoteTargetsContainerFromFieldPath(t *testing.T) {
	ev := warningEvent("Pod", "spec.containers{app}")
	observation, ok := kube.EventNote(ev, fixedTime())
	assert.True(t, ok)
	assert.Equal(t, kube.ContainerID(testNamespace, "p", "app"),
		observation.Entity)
	assert.Equal(t, inventory.Noted, observation.Kind)
	assert.Equal(t, 3, observation.Note.Count)
	assert.Equal(t, "kubelet", observation.Note.Source)
	assert.True(t, observation.Note.Warning)
}

func TestEventNoteUsesSeries(t *testing.T) {
	seen := fixedTime().Add(time.Hour)
	ev := warningEvent("Node", "")
	ev.ReportingController = "node-controller"
	ev.Series = &corev1.EventSeries{
		Count: 9, LastObservedTime: metav1.NewMicroTime(seen),
	}
	observation, ok := kube.EventNote(ev, fixedTime())
	assert.True(t, ok)
	assert.Equal(t, 9, observation.Note.Count)
	assert.Equal(t, "node-controller", observation.Note.Source)
	assert.True(t, seen.Equal(observation.Note.At))
}

func TestEventNoteTimestampPrecedence(t *testing.T) {
	base := fixedTime()
	tt := []struct {
		name string
		set  func(*corev1.Event)
		want time.Time
	}{
		{"last", func(e *corev1.Event) {
			e.LastTimestamp = metav1.NewTime(base.Add(1))
		}, base.Add(1)},
		{"event_time", func(e *corev1.Event) {
			e.EventTime = metav1.NewMicroTime(base.Add(2))
		}, base.Add(2)},
		{"first", func(e *corev1.Event) {
			e.FirstTimestamp = metav1.NewTime(base.Add(3))
		}, base.Add(3)},
		{"created", func(e *corev1.Event) {
			e.CreationTimestamp = metav1.NewTime(base.Add(4))
		}, base.Add(4)},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ev := warningEvent("Pod", "")
			tc.set(ev)
			observation, ok := kube.EventNote(ev, base)
			assert.True(t, ok)
			assert.True(t, tc.want.Equal(observation.Note.At))
		})
	}
}

func TestEventNoteCarriesTheObjectAndEventIdentity(t *testing.T) {
	ev := warningEvent("Pod", "")
	ev.InvolvedObject.UID = "pod-uid"
	ev.UID = "event-uid"
	observation, ok := kube.EventNote(ev, fixedTime())
	assert.True(t, ok)
	assert.Equal(t, "pod-uid", observation.Note.UID)
	assert.Equal(t, "event-uid", observation.Note.Origin)
}

func TestEventNoteWithoutAnEventUIDHasNoOrigin(t *testing.T) {
	ev := warningEvent("Pod", "")
	ev.Name = "p.17a"
	observation, _ := kube.EventNote(ev, fixedTime())
	assert.Empty(t, observation.Note.Origin)
}

// The kubelet's Unhealthy events get the topic of the probe they name,
// so one container's readiness and liveness failures stay apart.
func TestEventNoteTopicNamesTheProbe(t *testing.T) {
	cases := map[string]string{
		"Readiness probe failed: connect: connection refused": "readiness",
		"Liveness probe failed: HTTP probe failed with statuscode: 500": "" +
			"liveness",
		"Startup probe errored: rpc error": "startup",
		"something else":                   "",
	}
	for message, topic := range cases {
		ev := warningEvent("Pod", "spec.containers{app}")
		ev.Reason, ev.Message = "Unhealthy", message
		observation, ok := kube.EventNote(ev, fixedTime())
		assert.True(t, ok)
		assert.Equal(t, topic, observation.Note.Topic, message)
	}
	ev := warningEvent("Pod", "")
	ev.Message = "Readiness probe failed: refused"
	observation, _ := kube.EventNote(ev, fixedTime())
	assert.Empty(t, observation.Note.Topic, "only Unhealthy has topics")
}
