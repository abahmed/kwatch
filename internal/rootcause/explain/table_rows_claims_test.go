package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

var claimRowCases = []rowCase{
	{row: "resource-claim-unallocated",
		want: "resourceclaim.resource.k8s.io/ml/gpu-claim",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("ml", "trainer", 1)[0]
			f.fail(pod, detection.ModePending, degradedH, 2, "")
			claim := inventory.NewEntityID("resource.k8s.io",
				"resourceclaim", "ml", "gpu-claim")
			f.add(claim)
			f.relate(claim, inventory.OwnedBy, pod)
			f.fail(claim, detection.ModeDeviceUnallocated, failingH, 1, "")
			return pod
		}},
}

// A claim with no finding of its own is not blamed for a pending pod.
func TestClaimWithoutFindingIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	pod := f.workload("ml", "trainer", 1)[0]
	f.fail(pod, detection.ModePending, degradedH, 2, "")
	claim := inventory.NewEntityID("resource.k8s.io", "resourceclaim",
		"ml", "gpu-claim")
	f.add(claim)
	f.relate(claim, inventory.OwnedBy, pod)

	if c, ok := f.explain().CauseOf(pod); ok && c.Root == claim {
		t.Fatalf("a claim that reports nothing was blamed: %+v", c)
	}
}
