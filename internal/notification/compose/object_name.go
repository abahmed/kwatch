package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// objectName is how a message names id when it is the subject of a
// sentence. Most kinds are named by kindWord and name (placedName). A
// kind whose generated name says nothing is named by what it is about,
// from the facts the case holds: every sentence that names the object
// (lead, reminder, resolve, spread) goes through here, so one object
// never reads two ways.
func (f caseFacts) objectName(id inventory.EntityID) string {
	if name, ok := f.attachmentName(id); ok {
		return name
	}
	return placedName(id)
}

// attachmentName names a VolumeAttachment by its volume and node, since
// its own name is a hash: "attachment of volume pvc-1 to node n1".
func (f caseFacts) attachmentName(id inventory.EntityID) (string, bool) {
	if id.Kind != kube.KindVolumeAttachment {
		return "", false
	}
	for _, m := range f.members {
		if m.Entity != id || m.Reason != reasons.VolumeDetachFailure {
			continue
		}
		return attachedTo(m)
	}
	return "", false
}

// attachedTo is "attachment of volume V to node N" from a finding's
// evidence, when it names both.
func attachedTo(m detection.Finding) (string, bool) {
	volume := evidence(m, detection.EvidenceAttachVolume)
	node := evidence(m, detection.EvidenceAttachNode)
	if volume == "" || node == "" {
		return "", false
	}
	return "attachment of volume " + volume + " to node " + node, true
}
