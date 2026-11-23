package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var namedStorageRowCases = []rowCase{
	{row: "used-missing", want: "csidriver//hostpath.csi.example.com",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "db", 1)
			f.fail(pods[0], modeFailedMount, failingH, 2,
				"MountVolume.WaitForAttach failed: waiting for a volume "+
					"attachment for driver hostpath.csi.example.com: not found")
			return pods[0]
		}},
	{row: "used-missing",
		want: "volumesnapshotcontent.snapshot.storage.k8s.io//imported",
		build: func(f *fixture) inventory.EntityID {
			snap := inventory.NewEntityID(snapshotGroup, kindVolumeSnapshot,
				"shop", "restored")
			f.add(snap)
			f.fail(snap, "SnapshotContentMissing", failingH, 2,
				"VolumeSnapshotContent \"imported\" is missing")
			return snap
		}},
	{row: RuleAttacherDown, want: "deployment/kube-system/ebs-csi",
		build: func(f *fixture) inventory.EntityID {
			ctl := f.workload("kube-system", "ebs-csi", 1)
			f.observe(inventory.CoreID(kube.KindDeployment, "kube-system",
				"ebs-csi"), map[string]inventory.Value{
				kube.AttrReadyReplicas: inventory.Number(0)})
			f.fail(ctl[0], "NotReady", failingH, 1, "not ready")
			pods := f.workload("shop", "db", 1)
			f.fail(pods[0], modeFailedAttach, failingH, 2,
				"AttachVolume.Attach failed: timed out waiting for "+
					"external-attacher of ebs.csi.example.com CSI driver")
			return pods[0]
		}},
	{row: RuleClaimRestoresFromSnapshot,
		want: "persistentvolumeclaim/shop/data",
		build: func(f *fixture) inventory.EntityID {
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			f.observe(claim, map[string]inventory.Value{
				kube.AttrRestoreSnapshot: inventory.Text("nightly")})
			snap := inventory.NewEntityID(snapshotGroup, kindVolumeSnapshot,
				"shop", "nightly")
			f.add(snap)
			f.fail(snap, "StuckDeleting", failingH, 1,
				"Deletion is blocked by finalizers")
			return snap
		}},
	{row: RuleClaimSelectedNodeGone,
		want: "persistentvolumeclaim/shop/data",
		build: func(f *fixture) inventory.EntityID {
			return selectedNodeCase(f, "n9")
		}},
}

// selectedNodeCase has a pending claim that names selected, and a pod
// that waits for it.
func selectedNodeCase(f *fixture, selected string) inventory.EntityID {
	f.add(inventory.CoreID(kube.KindNode, "", "n1"))
	claim := inventory.CoreID(kube.KindPVC, "shop", "data")
	f.observe(claim, map[string]inventory.Value{
		kube.AttrPhase:        inventory.Text("Pending"),
		kube.AttrSelectedNode: inventory.Text(selected)})
	pods := f.workload("shop", "db", 1)
	f.relate(pods[0], inventory.Mounts, claim)
	f.fail(pods[0], "Unschedulable", failingH, 2,
		"0/1 nodes are available: 1 node(s) didn't find available "+
			"persistent volumes to bind.")
	return pods[0]
}

// TestSelectedNodeThatExistsIsNotBlamed: a claim waiting on a node that
// is in the cluster is only waiting.
func TestSelectedNodeThatExistsIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	effect := selectedNodeCase(f, "n1")
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Row == RuleClaimSelectedNodeGone {
		t.Fatalf("blamed %s for a node that exists", c.Root)
	}
}
