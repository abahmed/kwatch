package handler

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
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
		first := h.fs.caBlocked.mark(ev.Reason, h.now())
		if h.now().Sub(first) < caSustainedMinutes*time.Minute {
			return
		}

		hint := ev.Message
		if hint == "" {
			hint = "Cluster autoscaler cannot scale: " + ev.Reason
		}

		obs := observe.Synthetic(
			"cluster-autoscaler", "cluster-autoscaler", ev.Reason,
		).WithSeverity(model.SeverityWarning).WithHint(hint)
		obs.NodeName = ev.InvolvedObject.Name
		obs.Transient = true
		h.observe(obs)

	default:
		// TriggeredScaleUp, ScaleDown, etc. — informational. Any non-failure
		// CA event means the autoscaler is functioning again, so clear the
		// sustained gates for the failure reasons; otherwise a later failure
		// would alert immediately without a fresh sustain window.
		h.fs.caBlocked.clear("FailedToScaleUp")
		h.fs.caBlocked.clear("NotTriggerScaleUp")
		// The autoscaler working again is an observed recovery, and it was
		// the only one available: nothing else ever resolved these, so a
		// scale-up failure stayed open until the stale sweep closed it with
		// "not observed to recover" -- which was untrue.
		autoscaler := model.ObjectRef{
			Kind: "cluster-autoscaler", Name: "cluster-autoscaler",
		}
		h.correlator.Resolve(autoscaler, "FailedToScaleUp")
		h.correlator.Resolve(autoscaler, "NotTriggerScaleUp")
	}
}
