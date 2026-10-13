package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// memoryLimitPlace is the memory limit a container reaches soon after
// every start.
func (s Snapshot) memoryLimitPlace(c Cause) *detection.Place {
	for _, container := range s.containersOf(c) {
		if limit, ok := attrNumber(container, kube.AttrMemoryLimit); ok &&
			limit > 0 && shortLastRun(container) {
			return s.containerPlace(container, "resources.limits.memory",
				detection.MemoryText(limit))
		}
	}
	return nil
}

// ephemeralLimitPlace is the ephemeral storage limit a pod was evicted
// for.
func (s Snapshot) ephemeralLimitPlace(c Cause) *detection.Place {
	for _, container := range s.containersOf(c) {
		if limit, ok := attrNumber(container, kube.AttrEphemeralLimit); ok &&
			limit > 0 {
			return s.containerPlace(container,
				"resources.limits.ephemeral-storage",
				detection.MemoryText(limit))
		}
	}
	return nil
}
