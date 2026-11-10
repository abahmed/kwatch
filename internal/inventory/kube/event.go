package kube

import (
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// containerFieldPath extracts the container from an event's field path,
// such as "spec.containers{app}", "spec.initContainers{migrate}" or
// "spec.ephemeralContainers{debugger}".
var containerFieldPath = regexp.MustCompile(`^spec\.(?:initContainers|` +
	`containers|ephemeralContainers)\{([^}]+)\}$`)

// EventNote turns a Kubernetes Event into a note on the object it is
// about. Normal events are ignored: only warnings are evidence of
// failure.
func EventNote(obj any, now time.Time) (inventory.Observation, bool) {
	ev, ok := obj.(*corev1.Event)
	if !ok || !keepEvent(ev) {
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
	// Events written through events.k8s.io carry no count; one event
	// is still one occurrence.
	count = max(count, 1)
	uid := string(ref.UID)
	if ref.Kind == "Node" && uid == ref.Name {
		// The kubelet names the node by its name here, not its UID.
		uid = ""
	}
	return inventory.Observation{
		Kind: inventory.Noted, Source: ObservationSource, At: now, Entity: id,
		Note: inventory.Note{
			At: eventTime(ev), Source: eventSource(ev), Reason: ev.Reason,
			Topic:   eventTopic(ev),
			Message: eventText(ev.Message), Count: count,
			Warning: ev.Type == corev1.EventTypeWarning,
			UID:     uid, Origin: eventOrigin(ev),
			Refusal: PullRefusal(ev.Message),
		},
	}, true
}

// probeTopics are the topics of the kubelet's Unhealthy events, by the
// probe their message names ("Readiness probe failed: ...",
// pkg/kubelet/prober). One container's readiness and liveness failures
// are different news: a liveness event must not replace the count and
// message of the readiness ones.
var probeTopics = []struct{ prefix, topic string }{
	{"Readiness probe", "readiness"},
	{"Liveness probe", "liveness"},
	{"Startup probe", "startup"},
}

// eventTopic is the topic of an event whose reason covers several
// things, or "".
func eventTopic(ev *corev1.Event) string {
	if ev.Reason != "Unhealthy" {
		return ""
	}
	for _, p := range probeTopics {
		if strings.HasPrefix(ev.Message, p.prefix) {
			return p.topic
		}
	}
	return ""
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

// eventOrigin names the Event object by its UID, so repeated updates of
// one Event replace its count and different Events of one reason add up.
// An Event without a UID (only tests build those) has no origin and the
// latest note of its reason wins.
func eventOrigin(ev *corev1.Event) string {
	return string(ev.UID)
}

// keepEvent is true for every Warning event, and for the Normal events
// in which an autoscaler says what it decided for a pod, or the scheduler
// says which pod preempted another.
func keepEvent(ev *corev1.Event) bool {
	return ev.Type == corev1.EventTypeWarning ||
		autoscalerReason(ev.Reason) || ev.Reason == PreemptedReason
}
