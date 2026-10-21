package detectors

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kubeletSource is the source name of the kubelet's own events.
const kubeletSource = "kubelet"

// staleNodeNote reports a kubelet note about a node that is Ready now
// and became Ready after the note was written. The kubelet writes its
// start-up notices before its node turns Ready, and a note from before
// a recovery describes a node that is no longer in that state, so
// neither is a current finding. A node that is not Ready keeps all its
// notes: they may be why.
func staleNodeNote(e inventory.Entity, note inventory.Note) bool {
	if e.ID.Kind != kube.KindNode || note.Source != kubeletSource {
		return false
	}
	status, _, since := condition(e, "Ready")
	return status == "True" && !since.IsZero() && note.At.Before(since)
}
