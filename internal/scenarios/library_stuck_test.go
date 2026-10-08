package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// stuckScenarios are things that should have ended and did not: objects
// held by finalizers, a controller that stopped renewing its Lease and a
// CronJob whose earlier run never finishes.
func stuckScenarios() []scenario {
	return append([]scenario{
		namespaceStuckTerminating(), claimStuckDeleting(),
		leaseHolderCrashLoops(), leaseHolderGone(),
		cronJobBlockedByStuckRun(), podTerminatingOnLostNode(),
	}, finalizerScenarios()...)
}

// deleting marks an object as deleted since at, held by finalizers.
func deleting(meta *metav1.ObjectMeta, at time.Time, finalizers ...string) {
	stamp := metav1.NewTime(at)
	meta.DeletionTimestamp = &stamp
	meta.Finalizers = finalizers
}

// namespaceStuckTerminating: a namespace was deleted two hours ago and
// the namespace controller reports what keeps it alive.
func namespaceStuckTerminating() scenario {
	return scenario{
		expect: expectation{
			Name: "namespace-stuck-terminating",
			Description: "A namespace has been terminating for hours; " +
				"its conditions say a claim still has a finalizer.",
			Root: "namespace//old-shop", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			ns := &corev1.Namespace{ObjectMeta: c.meta("", "old-shop")}
			deleting(&ns.ObjectMeta, c.now.Add(-2*time.Hour))
			ns.Spec.Finalizers = []corev1.FinalizerName{"kubernetes"}
			ns.Status.Phase = corev1.NamespaceTerminating
			ns.Status.Conditions = []corev1.NamespaceCondition{{
				Type:   corev1.NamespaceFinalizersRemaining,
				Status: corev1.ConditionTrue, Reason: "SomeFinalizersRemain",
				Message: "Some content in the namespace has finalizers " +
					"remaining: kubernetes.io/pvc-protection in 3 " +
					"resource instances",
				LastTransitionTime: metav1.NewTime(
					c.now.Add(-2 * time.Hour)),
			}}
			c.list(ns)
			c.after(20 * time.Minute)
			c.update(ns.DeepCopy())
		},
	}
}

// claimStuckDeleting: a claim was deleted an hour ago but a pod still
// mounts it, so the protection finalizer keeps it.
func claimStuckDeleting() scenario {
	return scenario{
		expect: expectation{
			Name: "claim-stuck-deleting",
			Description: "A PersistentVolumeClaim deleted an hour ago is " +
				"held by its protection finalizer while a pod uses it.",
			Root: "persistentvolumeclaim/shop/data", Tier: "notify", MaxMessages: 2,
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			claim := storageBoundClaim(c, "shop", "data", "fast", "10Gi",
				"data-pv")
			deleting(&claim.ObjectMeta, c.now.Add(-time.Hour),
				"kubernetes.io/pvc-protection")
			w := c.deployment("shop", "db", "registry.example.com/db:1", 1)
			pod := w.pod(0, "n1")
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: "data", VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.
						PersistentVolumeClaimVolumeSource{
						ClaimName: claim.Name},
				}})
			c.list(claim, w.deployment, w.replicaSet, pod)
			c.after(20 * time.Minute)
			c.update(claim.DeepCopy())
		},
	}
}

// leaseObservation is one scan of a controller's Lease, as the prober
// reports it, linked to the pod named by its holder.
func leaseObservation(
	c *cluster, name, holder string, renewed time.Time,
	pod *corev1.Pod,
) []inventory.Observation {
	id := inventory.CoreID(kube.KindLease, c.n("kube-system"), c.n(name))
	out := []inventory.Observation{{
		Kind: inventory.Observed, Source: kube.ProbeSource, At: c.now,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrLeaseHolder:   inventory.Text(holder),
			kube.AttrLeaseRenewed:  inventory.Time(renewed),
			kube.AttrLeaseDuration: inventory.Number(15),
		},
	}}
	if pod != nil {
		out = append(out, inventory.Observation{
			Kind: inventory.Related, Source: kube.ProbeSource, At: c.now,
			Entity: id, Relation: inventory.References,
			Targets: []inventory.EntityID{
				inventory.CoreID(kube.KindPod, pod.Namespace, pod.Name)},
		})
	}
	return out
}

// leaseHolderCrashLoops: the cloud controller crash-loops and its Lease
// was last renewed twelve minutes ago. The crash loop is the incident;
// the Lease is part of it.
func leaseHolderCrashLoops() scenario {
	return scenario{
		expect: expectation{
			Name: "lease-holder-crash-loops",
			Description: "A controller's Lease is not renewed for twelve " +
				"minutes because its holder pod crash-loops.",
			Root: "deployment/kube-system/ccm", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("kube-system", "ccm",
				"registry.example.com/ccm:1", 1)
			pod := w.pod(0, "n1", crashLoop(1, "Error",
				"fatal: cannot reach the cloud API", 6))
			w.setReady(0)
			c.list(w.deployment, w.replicaSet, pod)
			renewed := c.now.Add(-12 * time.Minute)
			for range 4 {
				c.emit(leaseObservation(c, "cloud-controller",
					pod.Name+"_1a2b", renewed, pod)...)
				c.after(2 * time.Minute)
			}
		},
	}
}

// leaseHolderGone: an uninstalled controller left its Lease behind. No
// pod holds it any more; nothing is happening.
func leaseHolderGone() scenario {
	return scenario{
		expect: expectation{
			Name: "lease-holder-gone",
			Description: "A Lease nobody renews because its controller " +
				"was uninstalled: no pod holds it, nothing runs.",
			Quiet: true, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			renewed := c.now.Add(-72 * time.Hour)
			for range 4 {
				c.emit(leaseObservation(c, "old-operator",
					"old-operator-7d9f-x_1a2b", renewed, nil)...)
				c.after(2 * time.Minute)
			}
		},
	}
}

// cronJobBlockedByStuckRun: a half-hourly report with concurrencyPolicy
// Forbid; its run of three hours ago never finished because its pod
// cannot pull its image, so every later run is skipped.
func cronJobBlockedByStuckRun() scenario {
	return scenario{
		expect: expectation{
			Name: "cronjob-blocked-by-stuck-run",
			Description: "A Forbid CronJob skips its runs because the " +
				"previous run's pod has been in ImagePullBackOff for " +
				"three hours.",
			Root: "cronjob/reports/nightly-report", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cron := scheduleCronJob(c, "reports", "nightly-report",
				"*/30 * * * *")
			started := c.now.Add(-3 * time.Hour)
			cron.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
			last := metav1.NewTime(started)
			cron.Status.LastScheduleTime = &last
			job := activeJob(c, cron, "nightly-report-run", started)
			cron.Status.Active = []corev1.ObjectReference{{
				Kind: "Job", Namespace: job.Namespace, Name: job.Name}}
			pod := stuckJobPod(c, job, "nightly-report-run-x")
			c.list(cron, job, pod)
			for range 4 {
				c.after(35 * time.Minute)
				c.update(pod.DeepCopy())
			}
		},
	}
}

// stuckJobPod is the pod of job, unable to pull its image.
func stuckJobPod(c *cluster, job *batchv1.Job, name string) *corev1.Pod {
	yes := true
	meta := c.meta(job.Namespace, name)
	meta.Namespace = job.Namespace
	meta.CreationTimestamp = job.CreationTimestamp
	meta.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "batch/v1", Kind: "Job", Name: job.Name,
		UID: job.UID, Controller: &yes,
	}}
	pod := &corev1.Pod{ObjectMeta: meta,
		Spec: *job.Spec.Template.Spec.DeepCopy()}
	pod.Spec.NodeName = c.n("n1")
	running(c, pod)
	waiting("ImagePullBackOff", "Back-off pulling image")(c, pod)
	pod.Status.Phase = corev1.PodPending
	return pod
}

// podTerminatingOnLostNode: a node stops reporting and the pod the taint
// manager deleted stays Terminating, because no kubelet is left to end
// it. The node is the cause, not the pod.
func podTerminatingOnLostNode() scenario {
	return scenario{
		expect: expectation{
			Name: "pod-terminating-node-lost",
			Description: "A pod deleted from an unreachable node stays " +
				"Terminating for over ten minutes: the node is the cause.",
			Root: "node//n1", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/apps/svc"},
			Tail:         duration(25 * time.Minute),
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			w := c.deployment("apps", "svc", "registry.example.com/s:1", 2)
			c.list(w.deployment, w.replicaSet, w.pod(0, "n1"),
				w.pod(1, "n2"))
			c.after(time.Minute)
			nodeLose(c, n1)
			c.after(5 * time.Minute)
			c.update(w.pod(0, "n1", notReady, nodeTaintEvicted))
		},
	}
}
