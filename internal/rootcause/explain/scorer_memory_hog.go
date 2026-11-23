package explain

import (
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func init() {
	scorers = append(scorers,
		scorer{"memory-hog", scoreNodeExhausted})
}

// scoreNodeExhausted rejects a node as the cause of its own memory
// trouble when the kernel named a workload's process in its system
// OOM: the workload is the cause, and the node only the place it ran.
func scoreNodeExhausted(v *view, c *candidate) outcome {
	if c.id.Kind != kube.KindNode || len(v.memoryHogHops(c.id)) == 0 {
		return outcome{}
	}
	return outcome{veto: "a workload's process was killed for memory " +
		"on it"}
}
