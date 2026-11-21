package scenarios

import (
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// gap6bStorageScenarios are classes, claims and snapshots that cannot be
// provisioned, restored or deleted, as the CSI sidecars report them.
func gap6bStorageScenarios() []scenario {
	return []scenario{
		gap6bClassSecretMissing(), gap6bClassBadParameter(),
		gap6bSizeOutOfRange(), gap6bRestoreNotReady(),
		gap6bSnapshotClassAmbiguous(), gap6bSnapshotCreationFailed(),
		gap6bSnapshotControllerAbsent(), gap6bSnapshotInUse(),
		gap6bSnapshotContentMissing(),
	}
}

const gap6Unbound = "0/2 nodes are available: pod has unbound immediate " +
	"PersistentVolumeClaims. preemption: 0/2 nodes are available: 2 " +
	"Preemption is not helpful for scheduling."

// gap6Waiting lists two nodes and a Deployment of namespace whose only pod
// is Pending for claim.
func gap6Waiting(
	c *cluster, namespace, app string, claim *corev1.PersistentVolumeClaim,
) *workload {
	w := c.deployment(namespace, app, "registry.example.com/"+app+":1", 1)
	storageMount(w, claim.Name)
	w.setReady(0)
	c.create(w.objects())
	c.create(w.pod(0, "", pendingUnscheduled(gap6Unbound)))
	return w
}

// gap6Provisioning warns ProvisioningFailed on claim once a minute.
func gap6Provisioning(
	c *cluster, claim *corev1.PersistentVolumeClaim, message string,
	rounds int32,
) {
	for m := int32(1); m <= rounds; m++ {
		c.after(time.Minute)
		g2Event(c, claim, "PersistentVolumeClaim", "ProvisioningFailed",
			message, "ebs.csi.example.com_csi-controller", m)
	}
}

// gap6bClassSecretMissing: the class's provisioner-secret parameter names
// a Secret that was deleted; new claims of the class stay Pending.
func gap6bClassSecretMissing() scenario {
	return scenario{
		expect: g2Exp("storageclass-provisioner-secret-missing",
			"A StorageClass names a provisioner Secret that no longer "+
				"exists; new claims of the class are Pending with "+
				"ProvisioningFailed.",
			"storageclass//secure-fast", "notify", "node//n1",
			"deployment/shop/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(scheduleClass(c, "secure-fast", "Immediate"))
			claim := scheduleClaim(c, "shop", "data", "secure-fast")
			c.create(claim)
			gap6Waiting(c, "shop", "db", claim)
			gap6Provisioning(c, claim, "failed to provision volume with "+
				"StorageClass \"secure-fast\": rpc error: code = "+
				"InvalidArgument desc = failed to get secret reference: "+
				"secrets \"provisioner-creds\" not found", 6)
		},
	}
}

// gap6bClassBadParameter: a class was edited with a reserved parameter the
// provisioner rejects; claims of three namespaces stay Pending.
func gap6bClassBadParameter() scenario {
	return scenario{
		expect: g2Exp("storageclass-parameter-rejected-claims-pending",
			"A StorageClass carries a parameter the provisioner "+
				"rejects; the claims of three namespaces are Pending "+
				"with the same ProvisioningFailed message.",
			"storageclass//fast-bad", "notify", "node//n1",
			"deployment/shop/db", "deployment/billing/ledger",
			"deployment/media/store").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(scheduleClass(c, "fast-bad", "Immediate"))
			var claims []*corev1.PersistentVolumeClaim
			for _, app := range []string{"shop/db", "billing/ledger",
				"media/store"} {
				ns, name, _ := strings.Cut(app, "/")
				claim := scheduleClaim(c, ns, "data-"+name, "fast-bad")
				c.create(claim)
				gap6Waiting(c, ns, name, claim)
				claims = append(claims, claim)
			}
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				for _, claim := range claims {
					g2Event(c, claim, "PersistentVolumeClaim",
						"ProvisioningFailed", "failed to provision volume "+
							"with StorageClass \"fast-bad\": invalid "+
							"parameter \"csi.storage.k8s.io/pvc-name-"+
							"typo\": reserved prefix",
						"ebs.csi.example.com_csi-controller", m)
				}
			}
		},
	}
}

// gap6bSizeOutOfRange: a chart asks for 100Mi where the backend's minimum
// is 1Gi.
func gap6bSizeOutOfRange() scenario {
	return scenario{
		expect: g2Exp("pvc-request-below-driver-minimum-size",
			"A claim asks for 100Mi where the backend's minimum volume "+
				"is 1Gi; it is Pending with an OutOfRange "+
				"ProvisioningFailed message.",
			"persistentvolumeclaim/shop/cache-data", "notify",
			"storageclass//standard", "node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(scheduleClass(c, "standard", "Immediate"))
			claim := scheduleClaim(c, "shop", "cache-data", "standard")
			claim.Spec.Resources.Requests[corev1.ResourceStorage] =
				resource.MustParse("100Mi")
			c.create(claim)
			gap6Waiting(c, "shop", "cache", claim)
			gap6Provisioning(c, claim, "failed to provision volume with "+
				"StorageClass \"standard\": rpc error: code = OutOfRange "+
				"desc = requested size 104857600 is less than the "+
				"minimum 1073741824", 6)
		},
	}
}

// gap6Snapshot is a VolumeSnapshot of namespace shop named name.
func gap6Snapshot(
	c *cluster, name string, status map[string]any,
) *unstructured.Unstructured {
	return builtinObject(c, "snapshot.storage.k8s.io/v1", "VolumeSnapshot",
		"shop", name, map[string]any{"source": map[string]any{
			"persistentVolumeClaimName": c.n("data")}}, status)
}

// gap6bRestoreNotReady: a claim is restored from a snapshot that is not
// ready to use.
func gap6bRestoreNotReady() scenario {
	return scenario{
		expect: g2Exp("pvc-restore-from-snapshot-not-ready",
			"A claim restored from a VolumeSnapshot that is not Ready "+
				"stays Pending with ProvisioningFailed.",
			"persistentvolumeclaim/shop/restore-data", "notify",
			"node//n1", "deployment/shop/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(scheduleClass(c, "standard", "Immediate"))
			c.list(gap6Snapshot(c, "nightly", map[string]any{
				"readyToUse": false}))
			claim := scheduleClaim(c, "shop", "restore-data", "standard")
			group := "snapshot.storage.k8s.io"
			claim.Spec.DataSource = &corev1.TypedLocalObjectReference{
				APIGroup: &group, Kind: "VolumeSnapshot", Name: c.n("nightly")}
			c.create(claim)
			gap6Waiting(c, "shop", "db", claim)
			gap6Provisioning(c, claim, "failed to provision volume with "+
				"StorageClass \"standard\": error getting handle for "+
				"DataSource Type VolumeSnapshot by Name nightly: snapshot "+
				"nightly is not Ready", 6)
		},
	}
}

// gap6bSnapshotClassAmbiguous: two VolumeSnapshotClasses are default for
// the same driver, so a scheduled snapshot cannot pick one.
func gap6bSnapshotClassAmbiguous() scenario {
	e := g2Exp("volume-snapshot-two-default-snapshot-classes",
		"A VolumeSnapshot never starts because two snapshot classes "+
			"are default for its driver; the controller warns "+
			"SetDefaultSnapshotClassFailed.",
		"volumesnapshot.snapshot.storage.k8s.io/shop/nightly", "digest", "node//n1").
		tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{expect: e, build: func(c *cluster) {
		c.list(c.node("n1", "zone-a"))
		for _, name := range []string{"snap-a", "snap-b"} {
			class := builtinObject(c, "snapshot.storage.k8s.io/v1",
				"VolumeSnapshotClass", "", name, map[string]any{},
				map[string]any{})
			class.SetAnnotations(map[string]string{"snapshot.storage." +
				"kubernetes.io/is-default-class": "true"})
			c.list(class)
		}
		snap := gap6Snapshot(c, "nightly", map[string]any{})
		c.create(snap)
		for m := int32(1); m <= 5; m++ {
			c.after(5 * time.Minute)
			g2Event(c, snap, "VolumeSnapshot", "SetDefaultSnapshotClassFailed",
				"Failed to set default snapshot class with error cannot "+
					"find default snapshot class: 2 default snapshot "+
					"classes were found", "snapshot-controller", m)
		}
	}}
}

// gap6bSnapshotCreationFailed: the driver cannot create the snapshot; the
// content carries the error.
func gap6bSnapshotCreationFailed() scenario {
	return scenario{
		expect: g2Exp("volume-snapshot-content-creation-failed",
			"A VolumeSnapshot never becomes ready because the driver "+
				"fails to create it; the content's status carries the "+
				"error.",
			"volumesnapshotcontent.snapshot.storage.k8s.io//snapcontent-3f2a", "notify",
			"node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			content := builtinObject(c, "snapshot.storage.k8s.io/v1",
				"VolumeSnapshotContent", "", "snapcontent-3f2a",
				map[string]any{"driver": "ebs.csi.example.com"},
				map[string]any{"error": map[string]any{"message": "rpc " +
					"error: code = ResourceExhausted desc = snapshot " +
					"quota exceeded for volume"}})
			snap := gap6Snapshot(c, "backup-1", map[string]any{
				"readyToUse": false, "boundVolumeSnapshotContentName": c.n(
					"snapcontent-3f2a")})
			c.create(snap, content)
			for m := int32(1); m <= 5; m++ {
				c.after(2 * time.Minute)
				g2Event(c, content, "VolumeSnapshotContent",
					"SnapshotCreationFailed", "Failed to create snapshot: "+
						"rpc error: code = ResourceExhausted desc = "+
						"snapshot quota exceeded for volume",
					"csi-snapshotter", m)
			}
		},
	}
}

// gap6bSnapshotControllerAbsent: the snapshot controller is scaled to
// zero; the VolumeSnapshots a backup tool creates never get a status.
func gap6bSnapshotControllerAbsent() scenario {
	return scenario{
		expect: g2Exp("snapshot-controller-scaled-to-zero",
			"The snapshot controller Deployment is scaled to zero; "+
				"VolumeSnapshots created by a backup tool never get "+
				"a status or an event.",
			"deployment/kube-system/snapshot-controller", "notify",
			"node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			ctl := c.deployment("kube-system", "snapshot-controller",
				"registry.k8s.io/sig-storage/snapshot-controller:v8.0.1", 0)
			c.list(ctl.objects())
			for i := range 3 {
				c.after(10 * time.Minute)
				c.create(gap6Snapshot(c, "backup-"+itoa(i+1),
					map[string]any{}))
			}
			c.after(20 * time.Minute)
		},
	}
}

// gap6bSnapshotInUse: a VolumeSnapshot deleted with a claim still being
// restored from it stays Terminating, held by a finalizer.
func gap6bSnapshotInUse() scenario {
	return scenario{
		expect: g2Exp("volume-snapshot-delete-blocked-by-restoring-claim",
			"A deleted VolumeSnapshot stays Terminating because a "+
				"claim is still being restored from it.",
			"persistentvolumeclaim/shop/restore-data", "digest",
			"node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(scheduleClass(c, "standard", "Immediate"))
			claim := scheduleClaim(c, "shop", "restore-data", "standard")
			group := "snapshot.storage.k8s.io"
			claim.Spec.DataSource = &corev1.TypedLocalObjectReference{
				APIGroup: &group, Kind: "VolumeSnapshot", Name: c.n("nightly")}
			c.create(claim)
			snap := gap6Snapshot(c, "nightly", map[string]any{
				"readyToUse": true})
			snap.SetDeletionTimestamp(&metav1.Time{Time: c.now.Add(-time.Hour)})
			snap.SetFinalizers([]string{"snapshot.storage.kubernetes.io/" +
				"volumesnapshot-as-source-protection"})
			c.list(snap)
			for m := int32(1); m <= 4; m++ {
				c.after(15 * time.Minute)
				g2Event(c, snap, "VolumeSnapshot", "SnapshotDeleteError",
					"Failed to delete snapshot: snapshot nightly is in "+
						"use (being used to restore a PVC)",
					"snapshot-controller", m)
			}
		},
	}
}

// gap6bSnapshotContentMissing: a VolumeSnapshot recreated from a backup
// points at a content that does not exist.
func gap6bSnapshotContentMissing() scenario {
	return scenario{
		expect: g2Exp("volume-snapshot-content-missing",
			"A VolumeSnapshot recreated from a backup names a "+
				"VolumeSnapshotContent that does not exist; it is never "+
				"Ready (SnapshotContentMissing).",
			"volumesnapshotcontent.snapshot.storage.k8s.io//imported-content", "notify",
			"node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			snap := gap6Snapshot(c, "restored", map[string]any{
				"readyToUse": false})
			c.create(snap)
			for m := int32(1); m <= 5; m++ {
				c.after(3 * time.Minute)
				g2Event(c, snap, "VolumeSnapshot", "SnapshotContentMissing",
					"VolumeSnapshotContent \"imported-content\" is "+
						"missing", "snapshot-controller", m)
			}
		},
	}
}
