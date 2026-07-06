package handler

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	corev1 "k8s.io/api/core/v1"
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
			Severity: "warning",
			NodeName: ev.InvolvedObject.Name,
		})

	default:
		// TriggeredScaleUp, ScaleDown, etc. — informational, reset
		// any previous blocked state for this reason
		h.caMu.Lock()
		key := ev.Reason
		if strings.HasPrefix(ev.Reason, "FailedTo") || strings.HasPrefix(ev.Reason, "NotTrigger") {
			key = ev.Reason
		}
		delete(h.firstCaBlocked, key)
		h.caMu.Unlock()
	}
}
