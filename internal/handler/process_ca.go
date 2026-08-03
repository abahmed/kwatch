package handler

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

const caSustainedMinutes = 5

// ProcessClusterAutoscalerEvent handles a cluster-autoscaler event and
// creates an incident if the autoscaler reports a scale failure.
// Recognized event reasons:
//   - FailedToScaleUp — the autoscaler could not add nodes
//   - NotTriggerScaleUp — a pod could not be scheduled due to resource
//     constraints that the autoscaler could not resolve
//   - ScaleDown, TriggeredScaleUp — informational, no alert
func (h *handler) ProcessClusterAutoscalerEvent(ev *corev1.Event) {
	switch ev.Reason {
	case "FailedToScaleUp", "NotTriggerScaleUp":
		// sustain check: only alert if the same reason persists
		h.caMu.Lock()
		first, ok := h.firstCaBlocked[ev.Reason]
		if !ok {
			h.firstCaBlocked[ev.Reason] = h.now()
			h.caMu.Unlock()
			return
		}
		h.caMu.Unlock()

		if h.now().Sub(first) < caSustainedMinutes*time.Minute {
			return
		}

		hint := ev.Message
		if hint == "" {
			hint = "Cluster autoscaler cannot scale: " + ev.Reason
		}

		h.signalEvent(&event.Signal{
			Resource: "cluster-autoscaler",
			Reason:   ev.Reason,
			Hint:     hint,
			Owner:    "cluster-autoscaler",
			Severity: model.SeverityWarning,
			NodeName: ev.InvolvedObject.Name,
		})

	default:
		// TriggeredScaleUp, ScaleDown, etc. — informational. Any non-failure
		// CA event means the autoscaler is functioning again, so clear the
		// sustained gates for the failure reasons; otherwise a later failure
		// would alert immediately without a fresh sustain window.
		h.caMu.Lock()
		delete(h.firstCaBlocked, "FailedToScaleUp")
		delete(h.firstCaBlocked, "NotTriggerScaleUp")
		h.caMu.Unlock()
	}
}
