package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const notReadyFor = "has not been ready for "

// withReplicaCount says how many replicas are ready when the lead is a
// pod that has not been ready and the pipeline knows the workload's
// counts: "has no ready replicas (0 of 2)". The pod's wait is dropped:
// it is how long one pod has not been ready, not how long the workload
// has been short, and the model keeps no time for the workload's count.
// Without the counts, or for any other state, the state is returned as
// it is.
func withReplicaCount(f caseFacts, state string) string {
	if !strings.HasPrefix(state, notReadyFor) || f.replicas == nil ||
		f.lead.Entity.Kind != kube.KindPod || f.lead.Entity == f.p.Root {
		return state
	}
	ready, desired := strconv.Itoa(f.replicas.Ready),
		strconv.Itoa(f.replicas.Desired)
	if f.replicas.Ready == 0 {
		return "has no ready replicas (0 of " + desired + ")"
	}
	return "has only " + ready + " of " + desired + " replicas ready"
}
