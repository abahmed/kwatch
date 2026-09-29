package kube

import (
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// containerFieldPath extracts the container from an event's field path,
// such as "spec.containers{app}" or "spec.initContainers{migrate}".
var containerFieldPath = regexp.MustCompile(
	`^spec\.(?:initContainers|containers)\{([^}]+)\}$`)

// EventNote turns a Kubernetes Event into a note on the object it is
// about. Normal events are ignored: only warnings are evidence of
// failure.
func EventNote(obj any, now time.Time) (knowledge.Fact, bool) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	ev, ok := obj.(*corev1.Event)
	if !ok || ev.Type != corev1.EventTypeWarning {
		return knowledge.Fact{}, false
	}
	ref := ev.InvolvedObject
	id := knowledge.NewEntityID(KindFor(ref.Kind), ref.Namespace, ref.Name)
	if m := containerFieldPath.FindStringSubmatch(ref.FieldPath); m != nil &&
		ref.Kind == "Pod" {
		id = ContainerID(ref.Namespace, ref.Name, m[1])
	}
	count := int(ev.Count)
	if ev.Series != nil {
		count = int(ev.Series.Count)
	}
	return knowledge.Fact{
		Kind: knowledge.Noted, Source: FactSource, At: now, Entity: id,
		Note: knowledge.Note{
			At: eventTime(ev), Source: eventSource(ev), Reason: ev.Reason,
			Message: truncate(ev.Message), Count: count, Warning: true,
		},
	}, true
}

func eventTime(ev *corev1.Event) time.Time {
	switch {
	case ev.Series != nil && !ev.Series.LastObservedTime.IsZero():
		return ev.Series.LastObservedTime.Time
	case !ev.LastTimestamp.IsZero():
		return ev.LastTimestamp.Time
	case !ev.EventTime.IsZero():
		return ev.EventTime.Time
	case !ev.FirstTimestamp.IsZero():
		return ev.FirstTimestamp.Time
	default:
		return ev.CreationTimestamp.Time
	}
}

func eventSource(ev *corev1.Event) string {
	if ev.ReportingController != "" {
		return ev.ReportingController
	}
	return ev.Source.Component
}
