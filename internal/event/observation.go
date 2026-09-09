package event

import (
	"github.com/abahmed/kwatch/internal/model"
)

// FromObservation renders an observation as the event the pipeline carries.
//
// This is the one conversion. Producers must not assemble an Event
// themselves: every hand-written copy of this dropped a different field, and
// dropped it silently -- labels (which disabled that producer's silence
// rules), the pod UID (which the engine needs to recognise a replacement
// pod), the subject's own name (which left the alert unable to say what broke).
func FromObservation(obs *model.Observation) Event {
	if obs == nil {
		return Event{}
	}
	ev := Event{
		Resource:        obs.Subject.Kind,
		Namespace:       obs.Subject.Namespace,
		PodName:         obs.Subject.Name,
		PodUID:          obs.Pod.UID,
		PodLineageID:    obs.Pod.LineageID,
		PodGenerateName: obs.Pod.GenerateName,
		NodeName:        obs.NodeName,
		ContainerName:   obs.Container,
		Image:           obs.Image,
		Message:         obs.Message,
		Reason:          obs.Reason,
		Events:          obs.Events,
		Logs:            obs.Logs,
		Labels:          obs.Labels,
		OwnerKind:       obs.OwnerKind(),
		RestartCount:    int(obs.RestartCount),
		Hint:            obs.Hint,
		Facts:           obs.Facts,
		Severity:        obs.Severity,
		IncludeEvents:   obs.IncludeEvents,
		IncludeLogs:     obs.IncludeLogs,
		Transient:       obs.Transient,
	}
	if ev.Hint == "" {
		ev.Hint = obs.Message
	}
	return ev
}
