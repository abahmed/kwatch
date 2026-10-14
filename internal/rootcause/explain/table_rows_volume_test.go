package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const multiAttachText = `Multi-Attach error for volume "pvc-1" Volume ` +
	"is already exclusively attached to one node and can't be attached " +
	"to another"

// attrs sets attributes on an entity the fixture already holds.
func (f *fixture) attrs(id inventory.EntityID, a map[string]inventory.Value) {
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: a})
}

// heldCase: a new pod on n2 waits for a claim's volume that the old pod,
// on n1, still holds. It returns the new pod.
func heldCase(f *fixture) (newPod, oldPod inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2")
	claim := inventory.CoreID(kube.KindPVC, "shop", "data")
	f.add(claim)
	oldPod = f.workload("shop", "old", 1, nodes[0])[0]
	newPod = f.workload("shop", "api", 1, nodes[1])[0]
	f.relate(oldPod, inventory.Mounts, claim)
	f.relate(newPod, inventory.Mounts, claim)
	f.fail(newPod, detection.ModeVolumeMultiAttach, failingH, 2,
		multiAttachText)
	return newPod, oldPod
}

var volumeRowCases = []rowCase{
	{row: "volume-held-by-pod", want: "pod/shop/old-1-0",
		build: func(f *fixture) inventory.EntityID {
			newPod, _ := heldCase(f)
			return newPod
		}},
	{row: "volume-held-by-node", want: "node//n1",
		build: func(f *fixture) inventory.EntityID {
			nodes := f.nodes("zone-a", "n1", "n2")
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			volume := inventory.CoreID(kube.KindPV, "", "pv-1")
			attach := inventory.CoreID(kube.KindVolumeAttachment, "", "csi-1")
			f.add(claim, volume, attach)
			f.relate(claim, inventory.References, volume)
			f.relate(attach, inventory.References, volume)
			f.relate(attach, inventory.RunsOn, nodes[0])
			pod := f.workload("shop", "api", 1, nodes[1])[0]
			f.relate(pod, inventory.Mounts, claim)
			f.fail(pod, detection.ModeVolumeMultiAttach, failingH, 2,
				multiAttachText)
			return pod
		}},
	{row: "provisioner-down", want: "deployment/kube-system/ebs-csi-controller",
		build: func(f *fixture) inventory.EntityID {
			claim := provisionerClaim(f, "ebs.csi.example.com")
			f.workload("kube-system", "ebs-csi-controller", 2)
			deployment := inventory.CoreID(kube.KindDeployment,
				"kube-system", "ebs-csi-controller")
			f.attrs(deployment, map[string]inventory.Value{
				kube.AttrReadyReplicas: inventory.Number(0)})
			return claim
		}},
	{row: "provisioner-down-pending", want: "deployment/kube-system/ebs-csi",
		build: func(f *fixture) inventory.EntityID {
			claim := provisionerClaim(f, "ebs.csi.example.com")
			delete(f.findings, claim) // seen only from the waiting pod
			f.attrs(claim, map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Pending")})
			f.workload("kube-system", "ebs-csi", 1)
			f.attrs(inventory.CoreID(kube.KindDeployment, "kube-system",
				"ebs-csi"), map[string]inventory.Value{
				kube.AttrReadyReplicas: inventory.Number(0)})
			pod := f.workload("shop", "db", 1)[0]
			f.relate(pod, inventory.Mounts, claim)
			f.fail(pod, detection.ModePending, degradedH, 3,
				"pod has unbound immediate PersistentVolumeClaims")
			return pod
		}},
	{row: "claim-no-storage-class", want: "persistentvolumeclaim/shop/data-0",
		build: func(f *fixture) inventory.EntityID {
			claim := inventory.CoreID(kube.KindPVC, "shop", "data-0")
			f.add(claim)
			f.attrs(claim, map[string]inventory.Value{
				kube.AttrPhase:      inventory.Text("Pending"),
				kube.AttrClassUnset: inventory.Bool(true)})
			f.add(inventory.CoreID(kube.KindStorageClass, "", "slow"))
			f.fail(claim, detection.ModeClaimFailed, degradedH, 2, "")
			return claim
		}},
}

// provisionerClaim is a Pending claim of a class that names provisioner.
func provisionerClaim(f *fixture, provisioner string) inventory.EntityID {
	class := inventory.CoreID(kube.KindStorageClass, "", "ebs")
	claim := inventory.CoreID(kube.KindPVC, "shop", "data-0")
	f.add(class, claim)
	f.attrs(class, map[string]inventory.Value{
		kube.AttrProvisioner: inventory.Text(provisioner)})
	f.relate(claim, inventory.References, class)
	f.fail(claim, detection.ModeClaimFailed, degradedH, 2, "")
	return claim
}

// A class whose provisioner has a healthy workload is not blamed, and a
// default class means the claim's silence is not its fault.
func TestVolumeCausesStayQuietWhenHealthy(t *testing.T) {
	f := newFixture(t)
	claim := provisionerClaim(f, "ebs.csi.example.com")
	f.workload("kube-system", "ebs-csi-controller", 1)
	f.attrs(inventory.CoreID(kube.KindDeployment, "kube-system",
		"ebs-csi-controller"), map[string]inventory.Value{
		kube.AttrReadyReplicas: inventory.Number(1)})

	c, ok := f.explain().CauseOf(claim)
	if ok && c.Row == "provisioner-down" {
		t.Fatalf("blamed a ready provisioner: %v", c.Root)
	}
}

func TestClaimWithDefaultClassIsNotBlamedForNoClass(t *testing.T) {
	f := newFixture(t)
	claim := inventory.CoreID(kube.KindPVC, "shop", "data-0")
	f.add(claim)
	f.attrs(claim, map[string]inventory.Value{
		kube.AttrPhase:      inventory.Text("Pending"),
		kube.AttrClassUnset: inventory.Bool(true)})
	class := inventory.CoreID(kube.KindStorageClass, "", "std")
	f.add(class)
	f.attrs(class, map[string]inventory.Value{
		kube.AttrDefaultClass: inventory.Bool(true)})
	f.fail(claim, detection.ModeClaimFailed, degradedH, 2, "")

	if c, ok := f.explain().CauseOf(claim); ok &&
		c.Row == "claim-no-storage-class" {
		t.Fatalf("blamed a claim although a default class exists")
	}
}

func TestClaimWithSelectorIsNotBlamedForNoClass(t *testing.T) {
	f := newFixture(t)
	claim := inventory.CoreID(kube.KindPVC, "shop", "data-0")
	f.add(claim)
	f.attrs(claim, map[string]inventory.Value{
		kube.AttrPhase:       inventory.Text("Pending"),
		kube.AttrClassUnset:  inventory.Bool(true),
		kube.AttrHasSelector: inventory.Bool(true)})
	f.add(inventory.CoreID(kube.KindStorageClass, "", "slow"))
	f.fail(claim, detection.ModeClaimFailed, degradedH, 2, "")

	if c, ok := f.explain().CauseOf(claim); ok &&
		c.Row == "claim-no-storage-class" {
		t.Fatalf("blamed a claim that selects its volume by labels")
	}
}

// Look-alikes and mixed candidates are never blamed.
func TestProvisionerDownNeedsOneClearCandidate(t *testing.T) {
	down := func(f *fixture, name string, ready float64) {
		f.workload("kube-system", name, 1)
		f.attrs(inventory.CoreID(kube.KindDeployment, "kube-system", name),
			map[string]inventory.Value{
				kube.AttrReadyReplicas: inventory.Number(ready)})
	}
	cases := map[string]func(f *fixture){
		"look-alike name": func(f *fixture) {
			down(f, "ebs-billing-controller", 0)
		},
		"one of two ready": func(f *fixture) {
			down(f, "ebs-csi-controller", 0)
			down(f, "ebs-csi-snapshotter", 1)
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			claim := provisionerClaim(f, "ebs.csi.example.com")
			build(f)

			if c, ok := f.explain().CauseOf(claim); ok &&
				c.Row == "provisioner-down" {
				t.Fatalf("blamed %v", c.Root)
			}
		})
	}
}
