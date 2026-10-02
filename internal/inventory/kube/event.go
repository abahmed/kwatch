package kube

import (
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// containerFieldPath extracts the container from an event's field path,
// such as "spec.containers{app}" or "spec.initContainers{migrate}".
var containerFieldPath = regexp.MustCompile(
	`^spec\.(?:initContainers|containers)\{([^}]+)\}$`)

// EventNote turns a Kubernetes Event into a note on the object it is
// about. Normal events are ignored: only warnings are evidence of
// failure.
func EventNote(obj any, now time.Time) (inventory.Observation, bool) {
	ev, ok := obj.(*corev1.Event)
	if !ok || ev.Type != corev1.EventTypeWarning {
		return inventory.Observation{}, false
	}
	ref := ev.InvolvedObject
	id := EntityFor(ref.APIVersion, ref.Kind, ref.Namespace, ref.Name)
	if m := containerFieldPath.FindStringSubmatch(ref.FieldPath); m != nil &&
		ref.Kind == "Pod" {
		id = ContainerID(ref.Namespace, ref.Name, m[1])
	}
	count := int(ev.Count)
	if ev.Series != nil {
		count = int(ev.Series.Count)
	}
	return inventory.Observation{
		Kind: inventory.Noted, Source: ObservationSource, At: now, Entity: id,
		Note: inventory.Note{
			At: eventTime(ev), Source: eventSource(ev), Reason: ev.Reason,
			Message: evidenceText(ev.Message), Count: count, Warning: true,
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
