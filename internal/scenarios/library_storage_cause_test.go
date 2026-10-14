package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// storageCauseScenarios are volumes whose trouble is in what holds or
// serves them: the old pod of a rollout, the provisioner, the class.
func storageCauseScenarios() []scenario {
	return []scenario{
		multiAttachHeld(), multiAttachClears(), provisionerDown(),
		provisionerUnseen(), claimNoClass(),
		provisionerScaledToZero(), provisionerLookAlike(),
	}
}

const multiAttachMessage = "Multi-Attach error for volume \"pvc-data\" " +
	"Volume is already exclusively attached to one node and can't be " +
	"attached to another"

// rolloutOverVolume starts a rolling update of an api Deployment whose
// only pod holds a ReadWriteOnce claim on n1. The old pod is
// terminating and the new one waits on n2. It returns the workload and
// both pods.
func rolloutOverVolume(c *cluster) (*workload, *corev1.Pod, *corev1.Pod) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	claim := storageBoundClaim(c, "shop", "data", "fast", "20Gi", "pv-data")
	c.list(scheduleClass(c, "fast", storagev1.VolumeBindingImmediate),
		storageVolume(c, claim, "pv-data"), claim)
	w := c.deployment("shop", "api", "registry.example.com/api:1", 1)
	storageMount(w, claim.Name)
	c.list(w.objects())
	old := w.pod(0, "n1")
	c.list(old)
	c.after(time.Minute)
	w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/api:2"
	})
	w.setReady(0)
	c.update(w.objects())
	stamp := metav1.NewTime(c.now)
	grace := int64(30)
	old = last(c, old)
	old.DeletionTimestamp, old.DeletionGracePeriodSeconds = &stamp, &grace
	c.update(old)
	fresh := w.pod(0, "n2", waiting("ContainerCreating", ""))
	c.create(fresh)
	return w, old, fresh
}

// attachEvents repeats the attach-detach controller's multi-attach
// warning for fresh every 30 seconds, count times.
func attachEvents(c *cluster, fresh *corev1.Pod, count int32) {
	for n := int32(1); n <= count; n++ {
		c.after(30 * time.Second)
		c.warn(c.warningEvent(fresh, "Pod", "FailedAttachVolume",
			multiAttachMessage, "attachdetach-controller", n))
	}
}

// readyJustNow is a pod that started, and became ready, just now.
func readyJustNow(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	pod.Status.StartTime = &now
	for i := range pod.Status.Conditions {
		pod.Status.Conditions[i].LastTransitionTime = now
	}
	for i := range pod.Status.ContainerStatuses {
		if running := pod.Status.ContainerStatuses[i].State.Running; running != nil {
			running.StartedAt = now
		}
	}
}

// multiAttachHeld: a rolling update of a one-replica API that keeps its
// state on a ReadWriteOnce volume. The old pod sits Terminating on n1 and
// keeps the volume attached; the new pod on n2 waits for it. The old pod
// is the root: the volume cannot move until it is gone.
func multiAttachHeld() scenario {
	return scenario{
		expect: expectation{
			Name: "multi-attach-held-by-old-pod",
			Description: "A rollout's new pod cannot attach the " +
				"ReadWriteOnce volume the old, still terminating pod " +
				"holds on another node.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n2", "storageclass//fast"},
			Tail:         duration(time.Minute),
		},
		build: func(c *cluster) {
			_, _, fresh := rolloutOverVolume(c)
			attachEvents(c, fresh, 10)
		},
	}
}

// multiAttachClears: the same rollout, but the old pod finishes stopping
// within a minute and the new pod attaches the volume and starts. The
// overlap of a normal rolling update is not worth a message.
func multiAttachClears() scenario {
	return scenario{
		expect: expectation{
			Name: "multi-attach-clears-quickly",
			Description: "A rollout's new pod briefly sees a multi-attach " +
				"error while the old pod stops; the volume moves and the " +
				"pod starts.",
			MaxMessages: 0, Quiet: true,
			Tail: duration(5 * time.Minute),
		},
		build: func(c *cluster) {
			w, old, fresh := rolloutOverVolume(c)
			attachEvents(c, fresh, 2)
			c.remove(old)
			w.setReady(1)
			c.update(w.objects())
			c.update(w.pod(0, "n2", readyJustNow))
			c.after(time.Minute)
		},
	}
}

// csiController is a CSI controller Deployment in kube-system that
// serves the "ebs.csi.example.com" provisioner.
func csiController(c *cluster) *workload {
	return c.deployment("kube-system", "ebs-csi-controller",
		"registry.example.com/ebs-csi-driver:1.30", 2)
}

// provisionerDown: the CSI controller that provisions volumes of the
// "gp" class crash-loops, so the database's new claim stays Pending and
// its pod cannot be scheduled. The controller is the root, not the claim.
func provisionerDown() scenario {
	return scenario{
		expect: expectation{
			Name: "provisioner-controller-down",
			Description: "A claim stays Pending because the CSI " +
				"controller that serves its StorageClass's provisioner " +
				"has no ready pods.",
			Root: "deployment/kube-system/ebs-csi-controller",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "storageclass//gp"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			class := scheduleClass(c, "gp", storagev1.VolumeBindingImmediate)
			class.Provisioner = "ebs.csi.example.com"
			c.list(class)
			ctl := csiController(c)
			c.list(ctl.objects())
			c.list(ctl.pod(0, "n1"), ctl.pod(1, "n2"))
			c.after(time.Hour)
			claim := scheduleClaim(c, "data", "orders-db", "gp")
			c.create(claim)
			db := c.deployment("data", "orders-db",
				"registry.example.com/postgres:16", 1)
			storageMount(db, claim.Name)
			db.setReady(0)
			c.create(db.objects())
			c.create(db.pod(0, "", pendingUnscheduled("0/2 nodes are "+
				"available: pod has unbound immediate "+
				"PersistentVolumeClaims. preemption: 0/2 nodes are "+
				"available: 2 Preemption is not helpful for scheduling.")))
			message := "back-off 5m0s restarting failed container"
			for n := int32(1); n <= 6; n++ {
				c.after(time.Minute)
				for i := 0; i < 2; i++ {
					c.update(ctl.pod(i, []string{"n1", "n2"}[i],
						crashLoop(1, "Error", message, n)))
				}
				ctl.setReady(0)
				c.update(ctl.objects())
			}
		},
	}
}

// provisionerUnseen: a claim of an external provisioner fails to provision
// and kwatch finds no workload that serves the provisioner (it may run
// outside the cluster). The claim stays the root and its message says
// so, quoting the provisioner's error.
func provisionerUnseen() scenario {
	return scenario{
		expect: expectation{
			Name: "provisioner-no-workload-found",
			Description: "A claim cannot be provisioned and no workload " +
				"of the cluster serves its StorageClass's provisioner.",
			Root: "persistentvolumeclaim/data/orders-db",
			Tier: "notify", MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			class := scheduleClass(c, "managed",
				storagev1.VolumeBindingImmediate)
			class.Provisioner = "disk.csi.example.com"
			c.list(class)
			claim := scheduleClaim(c, "data", "orders-db", "managed")
			c.create(claim)
			provision := "failed to provision volume with StorageClass " +
				"\"managed\": rpc error: code = Internal desc = could not " +
				"create the disk"
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				c.warn(c.warningEvent(claim, "PersistentVolumeClaim",
					"ProvisioningFailed", provision,
					"disk.csi.example.com", m))
			}
		},
	}
}

// claimNoClass: a claim sets no storageClassName, the cluster has no
// default StorageClass and no static volume, so nothing ever binds it.
func claimNoClass() scenario {
	return scenario{
		expect: expectation{
			Name: "claim-no-storage-class",
			Description: "A claim that names no StorageClass stays " +
				"Pending in a cluster with no default class.",
			Root: "persistentvolumeclaim/data/cache-0",
			Tier: "notify", MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(scheduleClass(c, "fast", storagev1.VolumeBindingImmediate))
			claim := scheduleClaim(c, "data", "cache-0", "unused")
			claim.Spec.StorageClassName = nil
			c.create(claim)
			c.after(5 * time.Minute)
		},
	}
}
