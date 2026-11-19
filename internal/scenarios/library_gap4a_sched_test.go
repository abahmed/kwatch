package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// gap4aSchedScenarios are pods the scheduler cannot place because of a
// claim, a storage capacity, an affinity or a priority class.
func gap4aSchedScenarios() []scenario {
	return []scenario{
		gap4aRWOPConflict(), gap4aNoFreeStorage(), gap4aClaimDeleting(),
		gap4aVolumeTerminating(), gap4aPodAffinityNoMatch(),
		gap4aPreemptionNever(),
	}
}

// gap4aDB lists a database Deployment that mounts claim, with its pod on
// n1, and returns it.
func gap4aDB(c *cluster, claim *corev1.PersistentVolumeClaim) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "db", "registry.example.com/db:5", 1)
	storageMount(w, claim.Name)
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	return w
}

// gap4aRollOut starts a rollout of w; the new pod is Pending with message.
func gap4aRollOut(c *cluster, w *workload, message string) {
	c.after(10 * time.Minute)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{Name: "LOG", Value: "info"}}
	})
	d := w.deployment
	d.Status.Replicas, d.Status.UpdatedReplicas = 2, 1
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 1, 1
	d.Status.UnavailableReplicas = 1
	c.update(d)
	c.create(rs)
	c.create(w.pod(0, "", pendingUnscheduled(message)))
	pendingForMinutes(c, w, message, 8)
}

// gap4aRWOPConflict: a database mounts a ReadWriteOncePod claim; a rollout
// surges one pod, which cannot get the claim while the old pod holds it.
func gap4aRWOPConflict() scenario {
	return scenario{
		expect: g2Exp("rollout-read-write-once-pod-conflict",
			"A Deployment mounting a ReadWriteOncePod claim rolls out "+
				"with a surge pod; the new pod is Pending because the "+
				"old pod still uses the claim.",
			"deployment/shop/db", "notify", "node//n1", "node//n2").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			claim := storageBoundClaim(c, "shop", "pgdata", "fast", "20Gi",
				"pv-pgdata")
			claim.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOncePod}
			c.list(scheduleClass(c, "fast", "Immediate"),
				storageVolume(c, claim, "pv-pgdata"), claim)
			w := gap4aDB(c, claim)
			gap4aRollOut(c, w, "0/2 nodes are available: 2 node has pod "+
				"using PersistentVolumeClaim with the same name and "+
				"ReadWriteOncePod access mode. preemption: 0/2 nodes are "+
				"available: 2 No preemption victims found for incoming "+
				"pod.")
		},
	}
}

// gap4aNoFreeStorage: a claim asks for 500Gi from a local-volume class;
// every node reports less free capacity.
func gap4aNoFreeStorage() scenario {
	return scenario{
		expect: g2Exp("pvc-larger-than-any-node-capacity",
			"A WaitForFirstConsumer claim asks for more storage than any "+
				"node reports free; its pod is Pending with \"did not "+
				"have enough free storage\" on every node.",
			"persistentvolumeclaim/shop/scratch", "notify", "node//n1",
			"deployment/shop/etl").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			claim := scheduleClaim(c, "shop", "scratch", "local-lvm")
			c.list(scheduleClass(c, "local-lvm", "WaitForFirstConsumer"))
			c.create(claim)
			w := c.deployment("shop", "etl", "registry.example.com/etl:3", 1)
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/3 nodes are available: 3 node(s) did not have " +
				"enough free storage. preemption: 0/3 nodes are " +
				"available: 3 Preemption is not helpful for scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 8)
		},
	}
}

// gap4aClaimDeleting: someone deleted the claim while the old pod uses it;
// the protection finalizer keeps it Terminating and the replacement pod
// is refused for using a claim that is being deleted.
func gap4aClaimDeleting() scenario {
	return scenario{
		expect: g2Exp("pod-pending-claim-being-deleted",
			"A claim deleted while a pod uses it stays Terminating; the "+
				"replacement pod is Pending with \"persistentvolumeclaim "+
				"is being deleted\".",
			"persistentvolumeclaim/shop/pgdata", "notify", "node//n1",
			"node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			claim := storageBoundClaim(c, "shop", "pgdata", "fast", "20Gi",
				"pv-pgdata")
			deleting(&claim.ObjectMeta, c.now.Add(-time.Hour),
				"kubernetes.io/pvc-protection")
			c.list(scheduleClass(c, "fast", "Immediate"),
				storageVolume(c, claim, "pv-pgdata"), claim)
			w := gap4aDB(c, claim)
			gap4aRollOut(c, w, "0/2 nodes are available: persistent"+
				"volumeclaim \""+claim.Name+"\" is being deleted. "+
				"preemption: 0/2 nodes are available: 2 Preemption is "+
				"not helpful for scheduling.")
		},
	}
}

// gap4aVolumeTerminating: an operator deleted a bound PersistentVolume;
// the pv-protection finalizer keeps it while its claim is bound.
func gap4aVolumeTerminating() scenario {
	return scenario{
		expect: g2Exp("pv-terminating-while-claim-bound",
			"A PersistentVolume deleted while its claim is bound stays "+
				"Terminating under the pv-protection finalizer.",
			"persistentvolumeclaim/shop/pgdata", "digest", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			claim := storageBoundClaim(c, "shop", "pgdata", "fast", "20Gi",
				"pv-pgdata")
			pv := storageVolume(c, claim, "pv-pgdata")
			c.list(scheduleClass(c, "fast", "Immediate"), pv, claim)
			gap4aDB(c, claim)
			c.after(10 * time.Minute)
			terminating := last(c, pv)
			deleting(&terminating.ObjectMeta, c.now,
				"kubernetes.io/pv-protection")
			c.update(terminating)
			for range 4 {
				c.after(10 * time.Minute)
				c.update(terminating.DeepCopy())
			}
		},
	}
}

// gap4aPodAffinityNoMatch: a release makes api require a node that runs a
// cache pod; the cache Deployment was scaled to zero.
func gap4aPodAffinityNoMatch() scenario {
	return g2Release(g2Exp("release-pod-affinity-target-scaled-to-zero",
		"A release requires scheduling next to pods labelled app=cache; "+
			"the cache Deployment is scaled to zero and the new pod is "+
			"Pending with \"didn't match pod affinity rules\".",
		"deployment/shop/cache", "notify", "node//n1", "node//n2").
		tail(10*time.Minute),
		func(c *cluster, spec *corev1.PodSpec) {
			spec.Affinity = &corev1.Affinity{PodAffinity: &corev1.PodAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.
					PodAffinityTerm{{TopologyKey: corev1.LabelHostname,
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": c.n("cache")}}}},
			}}
		},
		func(c *cluster, w *workload) {
			cache := c.deployment("shop", "cache",
				"registry.example.com/cache:4", 0)
			c.list(cache.objects())
			g2Pending("0/2 nodes are available: 2 node(s) didn't match "+
				"pod affinity rules. preemption: 0/2 nodes are "+
				"available: 2 Preemption is not helpful for scheduling.")(
				c, w)
		})
}

// gap4aPreemptionNever: a batch pod's PriorityClass never preempts; the
// cluster is full of lower-priority pods it could replace.
func gap4aPreemptionNever() scenario {
	return g2Release(g2Exp("pod-priority-class-preemption-never",
		"A pod whose PriorityClass has preemptionPolicy Never waits on a "+
			"full cluster although lower-priority pods run there.",
		"priorityclass//batch-nopreempt", "notify", "node//n1",
		"node//n2", "deployment/shop/api").tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.PriorityClassName = "batch-nopreempt"
		},
		func(c *cluster, w *workload) {
			class := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion":       "scheduling.k8s.io/v1",
				"kind":             "PriorityClass",
				"value":            int64(1000),
				"preemptionPolicy": "Never"}}
			class.SetName("batch-nopreempt")
			class.SetUID("priorityclass-batch-nopreempt")
			c.list(class)
			g2Pending("0/2 nodes are available: 2 Insufficient cpu. "+
				"preemption: not eligible due to preemptionPolicy=Never.")(
				c, w)
		})
}
