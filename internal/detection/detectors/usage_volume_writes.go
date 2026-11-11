package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// volumeFull is the share of bytes or inodes at which a volume is full:
// writes fail now, whatever its pods say.
const volumeFull = 99.0

// noSpace are the error texts of a write that failed for lack of space.
var noSpace = []string{"no space left on device", "disk quota exceeded"}

// volumeWritesFail reports a claim whose writes fail now: it is full, or
// a container of a pod that mounts it last failed with no space left.
func volumeWritesFail(ctx detection.Context, e inventory.Entity) bool {
	for _, attr := range []string{kube.AttrVolumeUsedPct,
		kube.AttrVolumeInodesPct} {
		if pct, ok := number(e, attr); ok && pct >= volumeFull {
			return true
		}
	}
	if ctx.Model == nil {
		return false
	}
	for _, pod := range ctx.Model.Related(e.ID, inventory.Mounts,
		inventory.Incoming) {
		for _, id := range ctx.Model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			c, ok := ctx.Model.Entity(id)
			if ok && saysNoSpace(c) {
				return true
			}
		}
	}
	return false
}

// saysNoSpace reports a container whose last error says no space left.
func saysNoSpace(c inventory.Entity) bool {
	for _, attr := range []string{kube.AttrLastErrorLine,
		kube.AttrLastMessage} {
		line := strings.ToLower(text(c, attr))
		for _, s := range noSpace {
			if strings.Contains(line, s) {
				return true
			}
		}
	}
	return false
}
