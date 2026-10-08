package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// attachmentLead names a stuck VolumeAttachment by what matters, since
// its own name is a hash: the volume, the node and the driver, then why
// it cannot finish. "Attachment of volume pvc-1 to node n1 by
// ebs.csi.example.com is stuck deleting; the volume pvc-1 it attaches
// no longer exists."
func attachmentLead(f caseFacts) (string, bool) {
	if f.p.Root.Kind != kube.KindVolumeAttachment || !f.ok ||
		f.lead.Reason != reasons.VolumeDetachFailure {
		return "", false
	}
	volume := evidence(f.lead, detection.EvidenceAttachVolume)
	node := evidence(f.lead, detection.EvidenceAttachNode)
	gone := evidence(f.lead, detection.EvidenceAttachGone)
	if volume == "" || node == "" || gone == "" {
		return "", false
	}
	text := "attachment of volume " + volume + " to node " + node
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
	return text + "the volume " + volume +
		" it attaches and the node no longer exist", true
}
