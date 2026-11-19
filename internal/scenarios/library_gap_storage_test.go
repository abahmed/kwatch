package scenarios

import (
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gapStorageScenarios are volumes that cannot grow or bind, jobs that
// run out of time and pods that cannot be deleted.
func gapStorageScenarios() []scenario {
	return []scenario{
		gapVolumeResizeFailed(), gapReleasedVolume(),
		gapCronDeadlineExceeded(), gapPodStuckTerminating(),
	}
}

// gapVolumeResizeFailed: a database's 20Gi claim fills up and is edited
// to 100Gi; the external resizer fails on every attempt, the claim keeps
// its old capacity and the database keeps crashing. The claim is the root.
func gapVolumeResizeFailed() scenario {
	return scenario{
		expect: expectation{
			Name: "pvc-volume-resize-failed",
			Description: "A full claim is edited to a larger size; the " +
				"CSI resizer reports VolumeResizeFailed on every try and " +
				"the claim keeps its old capacity.",
			Root: "persistentvolumeclaim/inventory/pgdata", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n1",
				"storageclass//standard"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			claim := storageBoundClaim(c, "inventory", "pgdata", "standard",
				"20Gi", "pv-pgdata")
			c.list(scheduleClass(c, "standard", "Immediate"),
				storageVolume(c, claim, "pv-pgdata"), claim)
			w := c.deployment("inventory", "postgres",
				"registry.example.com/postgres:16.4", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(3 * time.Minute)
			full := "PANIC:  could not write to file " +
				"\"pg_wal/xlogtemp.31\": No space left on device"
			storageCrashLoop(c, w, "n1", full, 3)
			edited := last(c, claim)
			edited.Spec.Resources.Requests[corev1.ResourceStorage] =
				resource.MustParse("100Gi")
			message := "resize volume \"pv-pgdata\" by resizer " +
				"\"ebs.csi.aws.com\" failed: rpc error: code = " +
				"Internal desc = Could not resize volume " +
				"\"vol-0a1b2c3d\": VolumeModificationRateExceeded: " +
				"You've reached the maximum modification rate per " +
				"volume limit."
			edited.Status.Conditions = []corev1.
				PersistentVolumeClaimCondition{{
				Type: "ControllerResizeError", Status: corev1.ConditionTrue,
				Message:            message,
				LastTransitionTime: metav1.NewTime(c.now),
			}}
			c.update(edited)
			for m := int32(1); m <= 5; m++ {
				c.warn(c.warningEvent(edited, "PersistentVolumeClaim",
					"VolumeResizeFailed", message, "external-resizer", m))
				c.update(w.pod(0, "n1", crashLoop(1, "Error", full, 3+m)))
				c.after(time.Minute)
			}
		},
	}
}

// gapReleasedVolume: a database's StatefulSet was recreated; its new
// claim cannot bind because the only volume of the class is Released,
// still carrying the old claim's reference under reclaimPolicy Retain.
// The volume is the cause, not the new claim.
func gapReleasedVolume() scenario {
	return scenario{
		expect: expectation{
			Name: "pv-released-blocks-claim",
			Description: "A new claim stays Pending because the only " +
				"matching PersistentVolume is Released and still " +
				"references the deleted claim.",
			Root: "persistentvolume//pv-db-0", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1",
				"deployment/ledger/db"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			old := storageBoundClaim(c, "ledger", "data-db-0", "manual",
				"20Gi", "pv-db-0")
			pv := storageVolume(c, old, "pv-db-0")
			pv.Spec.PersistentVolumeReclaimPolicy =
				corev1.PersistentVolumeReclaimRetain
			pv.Spec.ClaimRef.UID = "uid-of-the-deleted-claim"
			pv.Status.Phase = corev1.VolumeReleased
			claim := scheduleClaim(c, "ledger", "data-db-0", "manual")
			c.list(scheduleClass(c, "manual", "Immediate"), pv)
			c.create(claim)
			w := c.deployment("ledger", "db", "registry.example.com/db:5", 1)
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: pod has unbound " +
				"immediate PersistentVolumeClaims. preemption: 0/1 " +
				"nodes are available: 1 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				c.warn(c.warningEvent(claim, "PersistentVolumeClaim",
					"FailedBinding", "volume \""+c.n("pv-db-0")+
						"\" already bound to a different claim.",
					"persistentvolume-controller", m))
			}
		},
	}
}

// gapCronDeadlineExceeded: every run of an hourly CronJob outlives
// its activeDeadlineSeconds of two minutes and is failed with
// DeadlineExceeded. The CronJob's deadline is the cause, not the node.
func gapCronDeadlineExceeded() scenario {
	return scenario{
		expect: expectation{
			Name: "cron-active-deadline-exceeded",
			Description: "Three consecutive runs of a CronJob are failed " +
				"with DeadlineExceeded because they run longer than " +
				"activeDeadlineSeconds.",
			Root: "cronjob/ops/stats-rollup", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n1"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			// Hourly, at the minute after the scenario starts, so that
			// each run starts on its schedule whatever the clock.
			cron := scheduleCronJob(c, "ops", "stats-rollup",
				fmt.Sprintf("%d * * * *", (c.start.Minute()+1)%60))
			deadline := int64(120)
			cron.Spec.JobTemplate.Spec.ActiveDeadlineSeconds = &deadline
			c.list(cron)
			for run := range 3 {
				c.after(time.Minute)
				started := c.now
				job := activeJob(c, cron, "stats-rollup-2925"+itoa(run+10),
					started)
				job.Spec.ActiveDeadlineSeconds = &deadline
				scheduled := metav1.NewTime(started)
				edited := last(c, cron)
				edited.Status.LastScheduleTime = &scheduled
				edited.Status.Active = []corev1.ObjectReference{{
					Kind: "Job", Namespace: job.Namespace, Name: job.Name}}
				c.update(edited)
				c.create(job)
				c.after(2 * time.Minute)
				job = job.DeepCopy()
				job.Status.Active = 0
				job.Status.Conditions = []batchv1.JobCondition{{
					Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
					Reason:             "DeadlineExceeded",
					Message:            "Job was active longer than specified deadline",
					LastTransitionTime: metav1.NewTime(c.now),
				}}
				c.update(job)
				c.warn(c.warningEvent(job, "Job", "DeadlineExceeded",
					"Job was active longer than specified deadline",
					"job-controller", 1))
				edited = last(c, cron)
				edited.Status.Active = nil
				c.update(edited)
				c.after(57 * time.Minute)
			}
		},
	}
}

// gapPodStuckTerminating: a rolling update of a StatefulSet is waiting
// on broker 2, which was deleted forty minutes ago and is held by a
// finalizer that nothing removes. The finalizer is the cause, not the
// StatefulSet.
func gapPodStuckTerminating() scenario {
	finalizer := "storage.example.com/volume-detach"
	return scenario{
		expect: expectation{
			Name: "pod-stuck-terminating-finalizer",
			Description: "A StatefulSet update waits for a pod that has " +
				"been Terminating for forty minutes, held by a " +
				"finalizer.",
			Root: "finalizer/streaming/" + finalizer, Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{
				"statefulset/streaming/queue", "node//n1"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			sts := scaleQueue(c)
			c.after(time.Minute)
			updated := sts.DeepCopy()
			updated.Generation, updated.Status.ObservedGeneration = 2, 2
			updated.Status.UpdateRevision = "queue-b"
			updated.Status.UpdatedReplicas = 0
			updated.Status.ReadyReplicas = 2
			updated.Status.AvailableReplicas = 2
			c.update(updated)
			pod := operatorBroker(c, sts, 2, "n1", notReady)
			// Deletion was requested forty minutes ago, as the
			// description says.
			deleting(&pod.ObjectMeta, c.now.Add(-40*time.Minute), finalizer)
			c.update(pod)
			for range 4 {
				c.after(10 * time.Minute)
				c.update(pod.DeepCopy())
			}
		},
	}
}
