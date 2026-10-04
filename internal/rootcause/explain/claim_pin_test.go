package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// pinnedPod builds a StatefulSet pod that mounts a claim and that the
// scheduler rejects for a volume node affinity conflict.
func pinnedPod(f *fixture) (pod, claim inventory.EntityID) {
	pod = f.workload("streaming", "kafka", 1)[0]
	claim = inventory.CoreID(kube.KindPVC, "streaming", "data-kafka-0")
	f.add(claim)
	f.relate(pod, inventory.Mounts, claim)
	f.findings[pod] = append(f.findings[pod], detection.Finding{
		Entity: pod, Reason: "Unschedulable", Mode: detection.ModePending,
		Health: failingH, Since: t0.Add(2 * time.Minute),
		Summary: "Pod cannot be scheduled",
		Evidence: []detection.Evidence{{Label: "scheduler",
			Value: "0/3 nodes are available: 3 node(s) had volume node " +
				"affinity conflict."}},
	})
	return pod, claim
}

// The claim, not the scheduler, pins a pod that cannot follow its
// volume's zone.
func TestExplainClaimPinsPodOnVolumeAffinityConflict(t *testing.T) {
	f := newFixture(t)
	pod, claim := pinnedPod(f)

	c := requireCause(t, f.explain(), pod, claim.String())
	if c.Row != "claim-pins-pod" {
		t.Fatalf("row = %s, want claim-pins-pod", c.Row)
	}
}

// Another scheduling reason still blames the scheduler's constraint.
func TestExplainOtherSchedulingReasonsKeepTheScheduler(t *testing.T) {
	f := newFixture(t)
	pod := f.workload("shop", "api", 1)[0]
	f.findings[pod] = append(f.findings[pod], detection.Finding{
		Entity: pod, Reason: "Unschedulable", Mode: detection.ModePending,
		Health: failingH, Since: t0.Add(2 * time.Minute),
		Summary: "Pod cannot be scheduled",
		Evidence: []detection.Evidence{{Label: "scheduler",
			Value: "0/3 nodes are available: 3 Insufficient cpu."}},
	})

	c := requireCause(t, f.explain(), pod, "scheduling//Insufficient cpu")
	if c.Row != "scheduler-capacity" {
		t.Fatalf("row = %s, want scheduler-capacity", c.Row)
	}
}
