package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var storageRowCases = []rowCase{
	{row: "storage-class-missing", want: "storageclass//fast-ssd",
		build: func(f *fixture) inventory.EntityID {
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			f.add(claim)
			f.relate(claim, inventory.References,
				inventory.CoreID(kube.KindStorageClass, "", "fast-ssd"))
			f.fail(claim, "ClaimFailed", failingH, 1,
				"storageclass.storage.k8s.io \"fast-ssd\" not found")
			return claim
		}},
	{row: "storage-class-missing-pending", want: "storageclass//fast-ssd",
		build: func(f *fixture) inventory.EntityID {
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			f.add(claim)
			f.relate(claim, inventory.References,
				inventory.CoreID(kube.KindStorageClass, "", "fast-ssd"))
			pods := f.workload("shop", "db", 1)
			f.relate(pods[0], inventory.Mounts, claim)
			f.fail(pods[0], "Unschedulable", failingH, 2,
				"pod has unbound immediate PersistentVolumeClaims")
			return pods[0]
		}},
	{row: "storage-class-rejects", want: "storageclass//fast",
		build: func(f *fixture) inventory.EntityID {
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			f.add(claim)
			f.relate(claim, inventory.References,
				inventory.CoreID(kube.KindStorageClass, "", "fast"))
			f.fail(claim, "ClaimFailed", failingH, 1,
				"failed to get secret reference: not found")
			return claim
		}},
	{row: "storage-class-rejects", want: "storageclass//fast",
		build: func(f *fixture) inventory.EntityID {
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			f.add(claim)
			f.relate(claim, inventory.References,
				inventory.CoreID(kube.KindStorageClass, "", "fast"))
			f.note(claim, "invalid parameter \"x\": reserved prefix")
			pods := f.workload("shop", "db", 1)
			f.relate(pods[0], inventory.Mounts, claim)
			f.fail(pods[0], "Unschedulable", failingH, 2,
				"pod has unbound immediate PersistentVolumeClaims")
			return pods[0]
		}},
	{row: "claim-pins-volume", want: "persistentvolumeclaim/shop/data",
		build: func(f *fixture) inventory.EntityID {
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			volume := inventory.CoreID(kube.KindPV, "", "pv-data")
			f.observe(claim, map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Bound")})
			f.add(volume)
			f.relate(claim, inventory.References, volume)
			f.fail(volume, detection.ModeStuckDeleting, failingH, 1,
				"Deletion is blocked by finalizers pv-protection")
			return volume
		}},
}
