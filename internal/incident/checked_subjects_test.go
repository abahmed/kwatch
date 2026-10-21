package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// checkedOf raises one failure whose cause was checked with the given
// replica comparisons and returns what the incident remembers.
func checkedOf(t *testing.T, checks ...explain.Comparison) []string {
	t.Helper()
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "api")
	node := entity(kube.KindNode, "n1")
	r.cause(pod, node, "node n1 is down")
	r.rule.compared = map[inventory.EntityID][]explain.Comparison{pod: checks}

	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Warning))

	cause := r.only().Cause
	require.NotNil(t, cause)
	return cause.Checked
}

func replicaCheck(workload, text string) explain.Comparison {
	return explain.Comparison{Code: rootcause.ProofReplicasEverywhere,
		Subject: entity(kube.KindDeployment, workload), Text: text}
}

// Two workloads whose replicas all fail read as one check twice unless
// each check says whose replicas it counts.
func TestCheckedChecksNameTheirWorkloadWhenSeveralFail(t *testing.T) {
	got := checkedOf(t,
		replicaCheck("api", "all 6 replicas fail, on 3 nodes"),
		replicaCheck("web", "all 5 replicas fail, on 3 nodes"))

	assert.Equal(t, []string{"all 6 replicas fail, on 3 nodes (api)",
		"all 5 replicas fail, on 3 nodes (web)"}, got)
}

// One workload keeps its check as it was: the message names it already.
func TestCheckedCheckOfOneWorkloadIsNotNamed(t *testing.T) {
	got := checkedOf(t,
		replicaCheck("api", "both replicas fail, on 2 nodes"))

	assert.Equal(t, []string{"both replicas fail, on 2 nodes"}, got)
}
