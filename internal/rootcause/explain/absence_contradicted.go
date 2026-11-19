package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// schedulerSaysOtherwise reports a claim that is absent from the model
// while the scheduler's verdict on the pod that mounts it names a reason
// that has nothing to do with claims: an absence is a cause only when the
// effect's error is the kind an absence produces. A scheduler that
// rejects every node for room or a node limit did not look for the claim.
func (v *view) schedulerSaysOtherwise(
	id, effect inventory.EntityID,
) bool {
	if id.Kind != kube.KindPVC {
		return false
	}
	text := strings.ToLower(v.text(effect))
	return strings.Contains(text, "nodes are available") &&
		!strings.Contains(text, "persistentvolumeclaim") &&
		!strings.Contains(text, "volume claim")
}
