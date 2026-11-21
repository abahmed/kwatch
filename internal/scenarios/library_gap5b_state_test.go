package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// gap5bStateScenarios are StatefulSets, volumes, jobs and disks whose
// state outlasts the change that caused it.
func gap5bStateScenarios() []scenario {
	return []scenario{
		gap5bRolloutWedged(), gap5bDeletedOpenFiles(), gap5bInodeExhaustion(),
		gap5bClaimHeldByJobPod(), gap5bSelectedNodeGone(),
		gap5bCronJobPileUp(), gap5bJobRestartsForever(),
		gap5bCrashLoopBlocksDrain(),
	}
}

// gap5bRolloutWedged: a StatefulSet is updated to an image that cannot be
// pulled; the template is corrected soon after but the pod already
// created from the broken one is not replaced, so the rollout stays
// behind it.
func gap5bRolloutWedged() scenario {
	return scenario{
		expect: g2Exp("statefulset-rollout-wedged-after-template-fix",
			"A StatefulSet is updated to an image that cannot be pulled; "+
				"the template is corrected but the broken pod is not "+
				"replaced and the rollout makes no progress.",
			"statefulset/streaming/queue", "notify",
			"registry//registry.example.com", "node//n1", "node//n2").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			c.after(5 * time.Minute)
			bad := sts.DeepCopy()
			bad.Spec.Template.Spec.Containers[0].Image =
				"registry.example.com/queue:4-bad"
			bad.Generation, bad.Status.ObservedGeneration = 2, 2
			bad.Status.UpdateRevision, bad.Status.UpdatedReplicas = "queue-b", 0
			c.update(bad)
			c.remove(operatorBroker(c, sts, 2, "n1"))
			message := "Failed to pull image \"registry.example.com/queue:" +
				"4-bad\": rpc error: code = NotFound desc = failed to " +
				"pull and unpack image: not found"
			for round := int32(1); round <= 12; round++ {
				reason := "ErrImagePull"
				if round%2 == 0 {
					reason = "ImagePullBackOff"
				}
				pod := operatorBroker(c, sts, 2, "n1", startedNow,
					gapPending(waiting(reason, message)))
				c.update(pod)
				gapKubelet(c, pod, "Failed", message, round)
				if round == 3 {
					fixed := bad.DeepCopy()
					fixed.Spec.Template.Spec.Containers[0].Image =
						"registry.example.com/queue:4"
					fixed.Generation, fixed.Status.ObservedGeneration = 3, 3
					fixed.Status.UpdateRevision = "queue-c"
					c.update(fixed)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap5bDeletedOpenFiles: a container wrote a huge file and deleted it but
// keeps it open; the disk of n1 is full while the pod's visible usage is
// small, and the kubelet evicts its neighbours first.
func gap5bDeletedOpenFiles() scenario {
	return scenario{
		expect: g2Exp("node-disk-full-by-deleted-open-file",
			"A node's disk is full from a deleted file a container "+
				"still holds open; the kubelet evicts neighbours that "+
				"use little.",
			"node//n1", "notify", "deployment/shop/orders",
			"deployment/shop/payments").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"orders", "payments"},
				[]string{"n1", "n2"})
			noisy := c.deployment("batch", "uploader",
				"registry.example.com/uploader:3", 1)
			c.list(noisy.objects())
			c.list(noisy.pod(0, "n1"))
			c.after(10 * time.Minute)
			g2NodeCondition(c, "n1", corev1.NodeDiskPressure,
				corev1.ConditionTrue, "KubeletHasDiskPressure",
				"kubelet has disk pressure")
			for i, w := range fleet {
				c.update(w.pod(0, "n1", evicted("The node was low on "+
					"resource: ephemeral-storage. Threshold quantity: "+
					"10%, available: 3%.")))
				w.setReady(1)
				c.update(w.objects())
				c.create(w.pod(2+i, "n2", startedNow))
				c.after(time.Minute)
			}
			c.after(8 * time.Minute)
		},
	}
}

// gap5bInodeExhaustion: four pods of a workload crash-loop on n1 for days
// and leave an empty file per restart; the node runs out of inodes and
// evicts its neighbours.
func gap5bInodeExhaustion() scenario {
	return scenario{
		expect: g2Exp("crash-looping-pods-exhaust-node-inodes",
			"Four pods crash-looping for days on one node leave an "+
				"empty file per restart; the node reports disk pressure "+
				"for inodes and evicts other pods.",
			"deployment/batch/flapper", "notify", "deployment/shop/orders",
			"deployment/shop/payments").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"orders", "payments"},
				[]string{"n1", "n2"})
			flapper := c.deployment("batch", "flapper",
				"registry.example.com/flapper:1", 4)
			c.list(flapper.objects())
			for i := range 4 {
				c.list(flapper.pod(i, "n1", crashLoop(1, "Error",
					"exec /app/run: exec format error", int32(2800+i))))
			}
			flapper.setReady(0)
			c.update(flapper.objects())
			c.after(10 * time.Minute)
			g2NodeCondition(c, "n1", corev1.NodeDiskPressure,
				corev1.ConditionTrue, "KubeletHasDiskPressure",
				"kubelet has disk pressure")
			for _, w := range fleet {
				c.update(w.pod(0, "n1", evicted("The node was low on "+
					"resource: inodes. Threshold quantity: 5%, "+
					"available: 1%.")))
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(8 * time.Minute)
		},
	}
}

// gap5bClaimHeldByJobPod: a claim deleted after a Job finished stays
// Terminating because the Job's completed pod, kept, still mounts it.
func gap5bClaimHeldByJobPod() scenario {
	return scenario{
		expect: g2Exp("claim-terminating-held-by-completed-job-pod",
			"A claim deleted after its Job finished stays Terminating "+
				"because the Job's Succeeded pod still references it.",
			"persistentvolumeclaim/batch/results", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			claim := storageBoundClaim(c, "batch", "results", "fast", "10Gi",
				"pv-results")
			deleting(&claim.ObjectMeta, c.now.Add(-time.Hour),
				"kubernetes.io/pvc-protection")
			c.list(scheduleClass(c, "fast", "Immediate"),
				storageVolume(c, claim, "pv-results"), claim)
			job := retryJob(c, "export", 0)
			job.Status.Active, job.Status.Succeeded = 0, 1
			pod := retryPod(c, job, 0, "")
			pod.Spec.NodeName = c.n("n1")
			pod.Status.Phase = corev1.PodSucceeded
			pod.Spec.Volumes = []corev1.Volume{{Name: "results",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.
						PersistentVolumeClaimVolumeSource{
						ClaimName: claim.Name}}}}
			c.list(job, pod)
			for range 4 {
				c.after(15 * time.Minute)
				c.update(claim.DeepCopy())
			}
		},
	}
}

// gap5bSelectedNodeGone: the scheduler picked n9 for a delayed-binding
// claim and the autoscaler removed n9 before the volume was provisioned.
func gap5bSelectedNodeGone() scenario {
	return scenario{
		expect: g2Exp("pvc-selected-node-removed-before-provisioning",
			"A delayed-binding claim names a selected node that no "+
				"longer exists; the claim and its pod stay Pending.",
			"persistentvolumeclaim/shop/data", "notify", "node//n1",
			"deployment/shop/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(scheduleClass(c, "fast", "WaitForFirstConsumer"))
			claim := scheduleClaim(c, "shop", "data", "fast")
			claim.Annotations = map[string]string{
				"volume.kubernetes.io/selected-node": c.n("n9")}
			c.create(claim)
			w := c.deployment("shop", "db", "registry.example.com/db:5", 1)
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/2 nodes are available: 2 node(s) didn't find " +
				"available persistent volumes to bind. preemption: 0/2 " +
				"nodes are available: 2 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 8)
		},
	}
}

// gap5bCronJobPileUp: a CronJob runs every minute with an image that does
// not exist and keeps every Job; the pods fill the nodes' pod capacity and
// other workloads cannot be placed.
func gap5bCronJobPileUp() scenario {
	return scenario{
		expect: g2Exp("cronjob-pull-failures-fill-pod-capacity",
			"A CronJob every minute with a missing image leaves a "+
				"Pending pod per run until the nodes are at their pod "+
				"limit and other workloads cannot be scheduled.",
			"cronjob/ops/sync-feed", "notify", "deployment/shop/web",
			"deployment/billing/invoices").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			names := []string{"n1", "n2"}
			for _, name := range names {
				node := c.node(name, "zone-a")
				node.Status.Allocatable[corev1.ResourcePods] =
					resource.MustParse("14")
				c.list(node)
			}
			victims := gap2Spread(c, []string{"shop/web", "billing/invoices"},
				names[:1])
			cron := scheduleCronJob(c, "ops", "sync-feed", "* * * * *")
			c.list(cron)
			message := "Back-off pulling image \"registry.example.com/" +
				"sync-feed:missing\""
			for i := range 22 {
				c.after(time.Minute)
				job := jobOf(c, cron, "sync-feed-"+itoa(i+100), c.now)
				job.Status.Active = 1
				c.create(job)
				pod := retryPod(c, job, 0, "")
				pod.Name = job.Name + "-p"
				pod.UID = pod.UID + "p"
				pod.Spec.NodeName = c.n(names[i%2])
				pod.Status.Phase = corev1.PodPending
				pod.Status.ContainerStatuses[0].State =
					corev1.ContainerState{Waiting: &corev1.
						ContainerStateWaiting{Reason: "ImagePullBackOff",
						Message: message}}
				c.create(pod)
			}
			pending := "0/2 nodes are available: 2 Too many pods. " +
				"preemption: 0/2 nodes are available: 2 No preemption " +
				"victims found for incoming pod."
			for _, w := range victims {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
				c.create(w.pod(1, "", pendingUnscheduled(pending)))
			}
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				for _, w := range victims {
					g2Event(c, last(c, w.pod(1, "")), "Pod",
						"FailedScheduling", pending, "default-scheduler", m)
				}
			}
		},
	}
}

// gap5bJobRestartsForever: a Job with restartPolicy OnFailure restarts its
// one container in place for ever; the Job never reaches its backoff
// limit and never fails.
func gap5bJobRestartsForever() scenario {
	return scenario{
		expect: g2Exp("job-on-failure-restarts-in-place-forever",
			"A Job whose pod restarts in place under OnFailure climbs to "+
				"dozens of restarts and never reports Failed, however "+
				"low its backoffLimit.",
			"job/batch/import", "notify", "node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			job := retryJob(c, "import", 2)
			job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
			c.create(job)
			for round := int32(1); round <= 8; round++ {
				pod := retryPod(c, job, 0, "")
				pod.Spec.NodeName = c.n("n1")
				pod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				crashLoop(1, "Error", "import failed: schema mismatch at "+
					"row 1", round*6)(c, pod)
				c.update(pod)
				c.after(2 * time.Minute)
			}
		},
	}
}

// gap5bCrashLoopBlocksDrain: three brokers crash-loop; their budget
// requires 75% healthy, so a drain of n1 cannot evict any of them.
func gap5bCrashLoopBlocksDrain() scenario {
	return scenario{
		expect: g2Exp("crash-looping-statefulset-blocks-drain",
			"A StatefulSet's pods crash-loop; its disruption budget "+
				"allows no eviction because too few are healthy, and a "+
				"node drain waits on them.",
			"statefulset/streaming/queue", "notify",
			"poddisruptionbudget/streaming/queue-pdb", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			min := intstr.FromString("75%")
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: clusterMeta(c, "streaming", "queue-pdb", "pdb"),
				Spec: policyv1.PodDisruptionBudgetSpec{MinAvailable: &min,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": c.n("queue")}}},
				Status: policyv1.PodDisruptionBudgetStatus{ExpectedPods: 3,
					DesiredHealthy: 3, CurrentHealthy: 3}}
			c.list(pdb)
			c.after(5 * time.Minute)
			message := "panic: failed to join cluster: cluster id mismatch"
			for restarts := int32(1); restarts <= 6; restarts++ {
				for i := range 3 {
					c.update(operatorBroker(c, sts, i, "n"+itoa(i%2+1),
						crashLoop(1, "Error", message, restarts)))
				}
				blocked := pdb.DeepCopy()
				blocked.Status.CurrentHealthy = 0
				c.update(blocked)
				if restarts == 3 {
					advCordon(c, c.node("n1", "zone-a"))
				}
				c.after(time.Minute)
			}
		},
	}
}
