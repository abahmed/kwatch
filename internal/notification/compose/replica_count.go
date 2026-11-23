package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const notReadyFor = "has not been ready for "

// withReplicaCount says how many replicas are ready when the lead is a
// pod that has not been ready and the pipeline knows the workload's
// counts: "has had no ready replicas (0 of 2) for three minutes". The
// wait stays the pod's own. Without the counts, or for any other state,
// the state is returned as it is.
func withReplicaCount(f caseFacts, state string) string {
	wait, ok := strings.CutPrefix(state, notReadyFor)
	if !ok || f.replicas == nil || f.lead.Entity.Kind != kube.KindPod ||
		f.lead.Entity == f.p.Root {
		return state
	}
	ready, desired := strconv.Itoa(f.replicas.Ready),
		strconv.Itoa(f.replicas.Desired)
	if f.replicas.Ready == 0 {
		return "has had no ready replicas (0 of " + desired + ") for " +
			wait
	}
	return "has had only " + ready + " of " + desired +
		" replicas ready for " + wait
}
