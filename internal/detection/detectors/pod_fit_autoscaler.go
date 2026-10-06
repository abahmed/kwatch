package detectors

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// scaleUpPatience is how long a node the autoscaler said it is adding
// is expected to take. While it runs the pending pod is not reported,
// as for a booting pool; if no node came by then, the autoscaler's own
// words go into the report. It matches cluster-autoscaler's default
// max-node-provision-time.
const scaleUpPatience = 15 * time.Minute

// autoscalerWindow is how far back the autoscaler's events are read.
const autoscalerWindow = time.Hour

// autoscalerSignal is the autoscaler's latest word about a pending pod,
// read from its Kubernetes events.
type autoscalerSignal struct {
	adding  bool
	message string
	at      time.Time
}

// addingReasons say a node is on its way; blockedReasons say none is.
// The first two of each are cluster-autoscaler's, the others Karpenter's
// and the general scaling failure events.
var (
	addingReasons  = []string{kube.ReasonTriggeredScaleUp, kube.ReasonNominated}
	blockedReasons = []string{kube.ReasonNotTriggerScaleUp,
		"FailedScaleUp", "FailedScaling"}
)

// autoscalerOf reads the newest autoscaler event on the pod within the
// last hour, if any.
func autoscalerOf(
	ctx detection.Context, pod inventory.EntityID,
) (autoscalerSignal, bool) {
	var latest autoscalerSignal
	found := false
	for _, note := range ctx.Model.Notes(pod, ctx.Now.Add(-autoscalerWindow)) {
		adding := slices.Contains(addingReasons, note.Reason)
		if !adding && !slices.Contains(blockedReasons, note.Reason) {
			continue
		}
		if !found || note.At.After(latest.at) {
			latest = autoscalerSignal{adding: adding,
				message: note.Message, at: note.At}
			found = true
		}
	}
	return latest, found
}

// autoscalerEvidence quotes what the autoscaler said about the pod.
func autoscalerEvidence(
	ctx detection.Context, pod inventory.EntityID,
) []detection.Evidence {
	signal, ok := autoscalerOf(ctx, pod)
	if !ok {
		return nil
	}
	state := detection.AutoscalerBlocked
	switch {
	case signal.adding && ctx.Now.Sub(signal.at) < scaleUpPatience:
		state = detection.AutoscalerAdding
	case signal.adding:
		state = detection.AutoscalerLate
	}
	return []detection.Evidence{
		{Label: detection.EvidenceAutoscaler, Value: state},
		{Label: detection.EvidenceAutoscalerSays, Value: signal.message},
	}
}

// scaleUpGrace is the extra wait for a pod while the autoscaler is
// adding a node for it, or zero. It asks to look again when the
// patience has run out, since nothing else wakes the pod up.
func scaleUpGrace(
	ctx detection.Context, pod inventory.Entity,
) time.Duration {
	if ctx.Model == nil {
		return 0
	}
	signal, ok := autoscalerOf(ctx, pod.ID)
	left := signal.at.Add(scaleUpPatience).Sub(ctx.Now)
	if !ok || !signal.adding || left <= 0 {
		return 0
	}
	ctx.RecheckAfter(left + time.Nanosecond)
	return scaleUpPatience
}
