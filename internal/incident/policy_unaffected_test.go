package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A pressure condition that no workload feels waits for the digest.
func TestNodePressureThatAffectsNothingWaitsForTheDigest(t *testing.T) {
	r := newRig(t, Config{})
	node := inventory.CoreID(kube.KindNode, "", "n1")
	r.raise(at(0), sig(node, reasons.MemoryPressure, detection.Critical))

	r.tick(at(DefaultSettle))
	assert.Equal(t, Digest, r.only().Tier)
	assert.Equal(t, levelUnaffected, r.m.levelOf(r.m.incidents[r.only().ID]))
}

// A node that is lost is never held back by it.
func TestLostNodeIsNotUnaffected(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), lostNode("n1"))

	assert.False(t, unaffected(r.m.incidents[r.only().ID]))
}

// hostedPod observes a pod on node n1 in phase, ready or not.
func hostedPod(
	t *testing.T, r *rig, name, phase string, ready bool,
) {
	t.Helper()
	pod := entity(kube.KindPod, name)
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: t0, Entity: pod,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase: inventory.Text(phase),
			kube.AttrReady: inventory.Bool(ready),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.relate(pod, inventory.RunsOn, inventory.CoreID(kube.KindNode, "", "n1"))
}

// A completed Job run or an evicted pod on the node is not ready, but
// it is not failing: the node's condition still affects nothing.
func TestFinishedPodsOnTheNodeAreNotFailing(t *testing.T) {
	r := newRig(t, Config{})
	node := inventory.CoreID(kube.KindNode, "", "n1")
	hostedPod(t, r, "backup-29", "Succeeded", false)
	hostedPod(t, r, "evicted", "Failed", false)
	assert.False(t, hostedFailing(r.model, node))

	hostedPod(t, r, "web-1", "Running", false)
	assert.True(t, hostedFailing(r.model, node))
}
