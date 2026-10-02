package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// waitingForDetach starts the attach-detach controller's FailedAttachVolume
// message when a ReadWriteOnce volume is still attached to another node
// (formerly "Multi-Attach error"). It usually clears once the old node
// detaches, so it is its own, milder failure.
const waitingForDetach = "Waiting for detach"

// storageEventSummaries describe the volume and device events.
var storageEventSummaries = map[string]string{
	reasons.FailedMapVolume: "Block volume cannot be mapped into the pod",
	reasons.FailedPrepareDynamicResources: "Device driver cannot " +
		"prepare the pod's claimed devices",
	reasons.ProvisioningFailed: "Volume cannot be provisioned",
	reasons.VolumeAttachWaiting: "Volume is still attached to another " +
		"node; waiting for it to detach",
}

// eventReason is the finding reason of a mapped event: its own reason,
// except a FailedAttachVolume that only waits for another node's detach.
func eventReason(note inventory.Note) string {
	if note.Reason == "FailedAttachVolume" &&
		strings.HasPrefix(note.Message, waitingForDetach) {
		return reasons.VolumeAttachWaiting
	}
	return note.Reason
}
