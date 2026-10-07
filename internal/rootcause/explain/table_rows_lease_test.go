package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// leaseCase builds a Lease whose holder pod crash loops.
func leaseCase(f *fixture) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, "kube-system", "ccm-7d9f-abc")
	lease := inventory.CoreID(kube.KindLease, "kube-system",
		"cloud-controller")
	f.add(pod, lease)
	f.relate(lease, inventory.References, pod)
	f.fail(pod, detection.ModeCrashLoop, failingH, 1, "")
	f.fail(lease, detection.ModeLeaseStale, degradedH, 2, "")
	return lease
}

var leaseRowCases = []rowCase{
	{row: "lease-holder-failing", want: "pod/kube-system/ccm-7d9f-abc",
		build: leaseCase},
}

// A holder with no finding of its own explains nothing: the stale Lease
// is then the root.
func TestLeaseHolderMustFailToBeBlamed(t *testing.T) {
	f := newFixture(t)
	pod := inventory.CoreID(kube.KindPod, "kube-system", "ccm-7d9f-abc")
	lease := inventory.CoreID(kube.KindLease, "kube-system",
		"cloud-controller")
	f.add(pod, lease)
	f.relate(lease, inventory.References, pod)
	f.fail(lease, detection.ModeLeaseStale, degradedH, 2, "")

	if c, ok := f.explain().CauseOf(lease); ok && c.Root == pod {
		t.Fatalf("a healthy holder must not be blamed: %+v", c)
	}
}
