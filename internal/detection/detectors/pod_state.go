package detectors

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// podTrouble names what keeps a pod from running: the waiting reason of
// one of its containers (ImagePullBackOff, CrashLoopBackOff), or the
// phase of a pod that is Pending or Failed. Empty when nothing stops it.
func podTrouble(model inventory.Reader, pod inventory.Entity) string {
	for _, id := range model.Related(pod.ID, inventory.PartOf,
		inventory.Incoming) {
		box, ok := model.Entity(id)
		reason := text(box, kube.AttrStateReason)
		if ok && text(box, kube.AttrState) == "waiting" && reason != "" {
			return reason
		}
	}
	switch phase := text(pod, kube.AttrPhase); phase {
	case "Pending", "Failed":
		return phase
	}
	return ""
}
