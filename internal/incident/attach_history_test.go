package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func evictedSig(pod inventory.EntityID) detection.Finding {
	return sig(pod, reasons.Evicted, detection.Warning)
}

// An evicted pod no cause explains, whose workload runs every replica it
// wants, is history: it opens no incident.
func TestEvictedPodOfAHealthyWorkloadIsHistory(t *testing.T) {
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 2, 0)

	r.raise(at(0), evictedSig(pod))

	assert.Empty(t, r.m.Export())
	assert.Empty(t, r.tick(at(DefaultSettle)))
}

// The same pod is a failure while the workload still lacks a replica.
func TestEvictedPodOfADegradedWorkloadIsAFailure(t *testing.T) {
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 1, 0)

	r.raise(at(0), evictedSig(pod))

	assert.Len(t, r.m.Export(), 1)
}

// While a node explains the eviction the pod stays in the node's
// incident; it leaves it silently once the pressure is gone and the
// replacement runs.
func TestEvictedPodLeavesTheNodeIncidentWhenItsCauseIsGone(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 0)
	node := inventory.CoreID(kube.KindNode, "", "n1")
	r.cause(pod, node, "node n1 is low on memory")
	r.raise(at(0), evictedSig(pod))
	r.tick(at(DefaultSettle))
	assert.Len(t, r.of(node).Members, 1)

	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: t0.Add(time.Minute),
		Entity: deploy, Attributes: map[string]inventory.Value{
			kube.AttrReplicas:      inventory.Number(2),
			kube.AttrReadyReplicas: inventory.Number(2)},
	})
	require.NoError(t, err)
	delete(r.rule.causes, pod)
	r.raise(at(5*time.Minute), evictedSig(pod))

	for _, p := range r.m.Incidents() {
		assert.Empty(t, p.Members, "no incident holds the pod")
		assert.Equal(t, node, p.Root, "none of the workload's own")
	}
}
