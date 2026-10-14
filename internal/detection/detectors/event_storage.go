package detectors

import (
	"regexp"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// waitingForDetach starts the attach-detach controller's FailedAttachVolume
// message when a ReadWriteOnce volume is still attached to another node
// (formerly "Multi-Attach error"). It usually clears once the old node
// detaches, so it is its own, milder failure.
const waitingForDetach = "Waiting for detach"

// multiAttachWords are what the attach-detach controller says when a
// ReadWriteOnce volume is still attached to, or used by a pod on,
// another node ("Multi-Attach error ... already exclusively attached
// to one node", "... already used by pod(s) ...").
var multiAttachWords = []string{
	"multi-attach error", "exclusively attached to one node",
	"already used by pod(s)",
}

// DefaultMultiAttachHold is how long a multi-attach error may last before
// it is reported. A rolling update briefly overlaps the old and the new
// pod: the old pod stops, the volume detaches and the new pod attaches it.
const DefaultMultiAttachHold = 2 * time.Minute

// storageEventSummaries describe the volume and device events.
var storageEventSummaries = map[string]string{
	reasons.FailedMapVolume: "Block volume cannot be mapped into the pod",
	reasons.FailedPrepareDynamicResources: "Device driver cannot " +
		"prepare the pod's claimed devices",
	reasons.ProvisioningFailed: "Volume cannot be provisioned",
	reasons.VolumeAttachWaiting: "Volume is still attached to another " +
		"node; waiting for it to detach",
	reasons.VolumeMultiAttach: "Volume is still attached to another " +
		"node and cannot be attached to this one",
}

// eventReason is the finding reason of a mapped event: its own reason,
// except a FailedAttachVolume that only waits for another node's detach.
func eventReason(note inventory.Note) string {
	if note.Reason == "FailedAttachVolume" &&
		strings.HasPrefix(note.Message, waitingForDetach) {
		return reasons.VolumeAttachWaiting
	}
	if note.Reason == "FailedAttachVolume" && multiAttach(note.Message) {
		return reasons.VolumeMultiAttach
	}
	return note.Reason
}

// multiAttach reports a message of the multi-attach error.
func multiAttach(message string) bool {
	lower := strings.ToLower(message)
	for _, words := range multiAttachWords {
		if strings.Contains(lower, words) {
			return true
		}
	}
	return false
}

// attachedVolume matches the volume a multi-attach message names.
var attachedVolume = regexp.MustCompile(`volume "([^"]+)"`)

// heldEvidence says what holds the volume of a multi-attach error: its
// name from the message, and the pod and node the model shows using it.
func heldEvidence(
	ctx detection.Context, e inventory.Entity, message string,
) []detection.Evidence {
	var out []detection.Evidence
	if m := attachedVolume.FindStringSubmatch(message); m != nil {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceHeldVolume, Value: m[1]})
	}
	if ctx.Model == nil {
		return out
	}
	holders := kube.VolumeHolders(ctx.Model, e.ID)
	if len(holders) == 0 {
		return out
	}
	holder := holders[0]
	out = append(out, detection.Evidence{
		Label: detection.EvidenceAttachNode, Value: holder.Node.Name})
	if !holder.Pod.IsZero() {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceHeldPod, Value: holder.Pod.Name})
	}
	if holder.Terminating {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceHeldPodEnding, Value: "terminating"})
	}
	return out
}
