package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
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
	fact, ok := kube.EventNote(ev, fixedTime())
	assert.True(t, ok)
	assert.Equal(t, kube.ContainerID(testNamespace, "p", "app"),
		fact.Entity)
	assert.Equal(t, knowledge.Noted, fact.Kind)
	assert.Equal(t, 3, fact.Note.Count)
	assert.Equal(t, "kubelet", fact.Note.Source)
	assert.True(t, fact.Note.Warning)
}

func TestEventNoteUnwrapsTombstoneAndUsesSeries(t *testing.T) {
	seen := fixedTime().Add(time.Hour)
	ev := warningEvent("Node", "")
	ev.ReportingController = "node-controller"
	ev.Series = &corev1.EventSeries{
		Count: 9, LastObservedTime: metav1.NewMicroTime(seen),
	}
	fact, ok := kube.EventNote(
		cache.DeletedFinalStateUnknown{Obj: ev}, fixedTime())
	assert.True(t, ok)
	assert.Equal(t, 9, fact.Note.Count)
	assert.Equal(t, "node-controller", fact.Note.Source)
	assert.True(t, seen.Equal(fact.Note.At))
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
			fact, ok := kube.EventNote(ev, base)
			assert.True(t, ok)
			assert.True(t, tc.want.Equal(fact.Note.At))
		})
	}
}
