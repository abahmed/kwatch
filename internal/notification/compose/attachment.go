package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// attachmentLead names a stuck VolumeAttachment by what matters, since
// its own name is a hash: the volume, the node and the driver, then why
// it cannot finish. "Attachment of volume pvc-1 to node n1 by
// ebs.csi.example.com is stuck deleting; the volume pvc-1 it attaches
// no longer exists." With both gone nothing is left to detach it.
func attachmentLead(f caseFacts) (string, bool) {
	if f.p.Root.Kind != kube.KindVolumeAttachment || !f.ok ||
		f.lead.Reason != reasons.VolumeDetachFailure {
		return "", false
	}
	volume := evidence(f.lead, detection.EvidenceAttachVolume)
	name, named := attachedTo(f.lead)
	gone := evidence(f.lead, detection.EvidenceAttachGone)
	if !named || gone == "" {
		return "", false
	}
	text := name
	if driver := evidence(f.lead, detection.EvidenceAttachDriver); driver !=
		"" {
		text += " by " + driver
	}
	text += f.clusterTag() + " is stuck deleting; "
	switch gone {
	case "volume":
		return text + "the volume " + volume +
			" it attaches no longer exists", true
	case "node":
		return text + "its node no longer exists", true
	}
	return text + "its node and volume no longer exist, so nothing is " +
		"left to detach it", true
}

// orphanedAttachment reports a stuck attachment whose node and volume
// are both gone: no driver will ever detach it, so removing its
// finalizer by hand is safe.
func orphanedAttachment(
	root inventory.EntityID, members []detection.Finding,
) (detection.Finding, bool) {
	if root.Kind != kube.KindVolumeAttachment {
		return detection.Finding{}, false
	}
	for _, m := range members {
		if m.Entity == root && m.Reason == reasons.VolumeDetachFailure &&
			evidence(m, detection.EvidenceAttachGone) == "volume and node" {
			return m, true
		}
	}
	return detection.Finding{}, false
}

// orphanedAttachmentSteps check that the node and the volume are gone,
// then remove the attachment's finalizer, the one change that ends it.
func orphanedAttachmentSteps(
	root inventory.EntityID, m detection.Finding,
) []notification.Step {
	var steps []notification.Step
	if node := evidence(m, detection.EvidenceAttachNode); node != "" {
		steps = append(steps, notification.Step{
			Text:    "Check that its node is gone",
			Command: "kubectl get node " + quote(node)})
	}
	if pv := evidence(m, detection.EvidenceAttachVolume); pv != "" {
		steps = append(steps, notification.Step{
			Text:    "Check that its volume is gone",
			Command: "kubectl get pv " + quote(pv)})
	}
	return append(steps, notification.Step{
		Text: "Remove its finalizer: with its node and volume gone, " +
			"nothing else removes it (changes the cluster)",
		Command: "kubectl patch volumeattachment " + quote(root.Name) +
			" --type=merge -p " +
			quote(`{"metadata":{"finalizers":null}}`),
		Mutating: true})
}
