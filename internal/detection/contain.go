package detection

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
)

// detectContained runs one detector for one entity. A panic is contained
// at this boundary: that detector yields nothing for this pass (so it
// neither raises nor clears a finding of its own) and every other
// detector and entity is still evaluated. The entity is evaluated again
// when it next changes.
func detectContained(
	ctx Context, detector Detector, entity inventory.Entity,
) (found []Finding) {
	defer func() {
		if r := recover(); r != nil {
			found = nil
			metrics.DefaultRegistry().IncPanic("detector")
			klog.ErrorS(nil, "detection: detector panicked",
				"component", "detection", "operation", "detect",
				"detector", detector.Name(), "kind", string(entity.ID.Kind),
				"panic", fmt.Sprint(r))
		}
	}()
	return detect(ctx, detector, entity)
}
