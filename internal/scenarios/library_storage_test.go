package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// storageScenarios are volumes that are bound and mounted but cannot be
// written to.
func storageScenarios() []scenario {
	return append([]scenario{
		claimFullWithVolume(), claimFullUnlistedVolume(), pvcFull(),
		claimPinsPod(),
	}, append(volumeFullScenarios(), pullLoginScenarios()...)...)
}

// claimFullWithVolume: a log store's 50Gi claim fills up. The store
// fails to flush its chunks with "no space left on device" and restarts
// again and again. The PersistentVolume behind the claim is listed and
// healthy: it is only the storage the claim asked for. The claim, the
// thing whose size a person changes, is the root.
func claimFullWithVolume() scenario {
	return scenario{
		expect: expectation{
			Name: "claim-full-with-volume",
			Description: "A log store's PersistentVolumeClaim fills up; " +
				"the store crash-loops with \"no space left on device\" " +
				"while its bound PersistentVolume is listed and healthy.",
			Root: "persistentvolumeclaim/observability/chunks",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//w3",
				"persistentvolume//pv-chunks-0",
				"storageclass//premium-ssd"},
		},
		build: func(c *cluster) {
			c.list(c.node("w3", "zone-c"))
			claim := storageBoundClaim(c, "observability", "chunks",
				"premium-ssd", "50Gi", "pv-chunks-0")
			c.list(scheduleClass(c, "premium-ssd", "Immediate"),
				storageVolume(c, claim, "pv-chunks-0"), claim)
			w := c.deployment("observability", "logstore",
				"registry.example.com/logstore:3.2", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "w3"))
			c.after(4 * time.Minute)
			message := "level=error msg=\"flush chunks\" err=\"write " +
				"/var/lib/logstore/chunks/1f3a: no space left on device\""
			storageCrashLoop(c, w, "w3", message, 6)
		},
	}
}

// claimFullUnlistedVolume: a queue broker's claim fills up; the broker
// exits with "No space left on device" on every start. kwatch sees the
// claim and the volume name it is bound to, but not the volume itself.
// The claim is still the root.
func claimFullUnlistedVolume() scenario {
	return scenario{
		expect: expectation{
			Name: "claim-full-unlisted-volume",
			Description: "A broker's bound PersistentVolumeClaim fills " +
				"up; the broker exits with \"No space left on device\" " +
				"and its PersistentVolume is not in view.",
			Root: "persistentvolumeclaim/messaging/broker-log",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//w1",
				"persistentvolume//pv-broker-log",
				"storageclass//fast-local"},
		},
		build: func(c *cluster) {
			c.list(c.node("w1", "zone-a"), c.node("w2", "zone-a"))
			claim := storageBoundClaim(c, "messaging", "broker-log",
				"fast-local", "8Gi", "pv-broker-log")
			c.list(scheduleClass(c, "fast-local", "WaitForFirstConsumer"),
				claim)
			w := c.deployment("messaging", "broker",
				"registry.example.com/broker:5.0", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "w1"))
			c.after(3 * time.Minute)
			message := "FATAL: log segment append failed: " +
				"/data/segments/000042.log: No space left on device"
			storageCrashLoop(c, w, "w1", message, 4)
		},
	}
}

// storageBoundClaim is a claim of class bound to volume with size.
func storageBoundClaim(c *cluster, namespace, name, class, size,
	volume string) *corev1.PersistentVolumeClaim {
	claim := scheduleClaim(c, namespace, name, class)
	quantity := resource.MustParse(size)
	claim.Spec.Resources.Requests[corev1.ResourceStorage] = quantity
	claim.Spec.VolumeName = c.n(volume)
	claim.Status.Phase = corev1.ClaimBound
	claim.Status.Capacity = corev1.ResourceList{
		corev1.ResourceStorage: quantity,
	}
	return claim
}

// storageVolume is the healthy, bound PersistentVolume of claim.
func storageVolume(c *cluster, claim *corev1.PersistentVolumeClaim,
	name string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: c.meta("", name),
		Spec: corev1.PersistentVolumeSpec{
			Capacity:         claim.Status.Capacity,
			StorageClassName: *claim.Spec.StorageClassName,
			ClaimRef: &corev1.ObjectReference{
				Kind: "PersistentVolumeClaim", Namespace: claim.Namespace,
				Name: claim.Name, UID: claim.UID,
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeBound,
		},
	}
}

// storageMount mounts claim as the workload's data volume.
func storageMount(w *workload, claim string) {
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Volumes = []corev1.Volume{{
			Name: "data", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.
					PersistentVolumeClaimVolumeSource{ClaimName: claim},
			},
		}}
	})
}

// storageCrashLoop restarts the workload's only pod on node with message
// once a minute, restarts times.
func storageCrashLoop(c *cluster, w *workload, node, message string,
	restarts int32) {
	for n := int32(1); n <= restarts; n++ {
		c.update(w.pod(0, node, crashLoop(1, "Error", message, n)))
		w.setReady(0)
		c.update(w.objects())
		c.after(time.Minute)
	}
}

// pvcFull: the database's 20Gi volume fills up. PostgreSQL cannot
// write its WAL and shuts down; on every restart it fails again with
// "No space left on device". The claim that is full is the root; the
// node and the StorageClass are fine.
// Held out until 2026-10-01 as
// heldout-pvc-full; labelled since its miss was looked at
// (SCORECARD.md, held-out rotation).
func pvcFull() scenario {
	return scenario{
		expect: expectation{
			Name: "pvc-full",
			Description: "A database's PersistentVolumeClaim fills up; " +
				"the database crash-loops with \"No space left on " +
				"device\" while writing its WAL.",
			Root: "persistentvolumeclaim/inventory/pgdata", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1",
				"storageclass//standard-rwo"},
		},
		build: buildPVCFull,
	}
}

func buildPVCFull(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	class := scheduleClass(c, "standard-rwo",
		"WaitForFirstConsumer")
	claim := scheduleClaim(c, "inventory", "pgdata", "standard-rwo")
	claim.Spec.VolumeName = "pvc-" + string(claim.UID)
	claim.Status.Phase = corev1.ClaimBound
	claim.Status.Capacity = claim.Spec.Resources.Requests
	w := c.deployment("inventory", "postgres",
		"registry.example.com/postgres:16.4", 1)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Volumes = []corev1.Volume{{
			Name: "data", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.
					PersistentVolumeClaimVolumeSource{ClaimName: claim.Name},
			},
		}}
	})
	c.list(class, claim)
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	c.after(5 * time.Minute)
	message := "PANIC:  could not write to file \"pg_wal/xlogtemp.31\": " +
		"No space left on device"
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(w.pod(0, "n1", crashLoop(1, "Error", message, restarts)))
		w.setReady(0)
		c.update(w.objects())
		c.after(time.Minute)
	}
}

// claimPinsPod: a broker's claim is bound to a volume in zone-a, and the
// only node left runs in zone-b. The scheduler rejects the replacement
// pod for a volume node affinity conflict. The claim is the cause.
func claimPinsPod() scenario {
	return scenario{
		expect: expectation{
			Name: "claim-pins-pod",
			Description: "A pod cannot be scheduled because its bound " +
				"claim's volume lives in a zone with no node for it.",
			Root: "persistentvolumeclaim/streaming/data-kafka-0",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n2", "deployment/streaming/kafka",
				"scheduling//had volume node affinity conflict"},
		},
		build: func(c *cluster) {
			c.list(c.node("n2", "zone-b"))
			claim := storageBoundClaim(c, "streaming", "data-kafka-0",
				"fast", "100Gi", "pv-kafka-0")
			c.list(claim, storageVolume(c, claim, "pv-kafka-0"))
			w := c.deployment("streaming", "kafka",
				"registry.example.com/kafka:3.7", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "", pendingUnscheduled("0/1 nodes are "+
				"available: 1 node(s) had volume node affinity conflict. "+
				"preemption: 0/1 nodes are available: 1 Preemption is not "+
				"helpful for scheduling.")))
			c.after(5 * time.Minute)
		},
	}
}
