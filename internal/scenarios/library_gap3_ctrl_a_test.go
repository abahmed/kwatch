package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// gap3CtrlAScenarios are DaemonSets, disruption budgets, Jobs and
// CronJobs whose controllers report why they do not do their work.
func gap3CtrlAScenarios() []scenario {
	return []scenario{
		gap3DaemonPlacement(), gap3DaemonPlacementBrief(),
		gap3DaemonFailedLoop(), gap3PDBNoPods(), gap3PDBUnmanaged(),
		gap3IndexedJob(), gap3PodFailurePolicy(), gap3TTLFailedJob(),
		gap3TTLCompletedJob(), gap3CronTimeZone(),
	}
}

// gap3Daemon lists a log shipper on four nodes.
func gap3Daemon(c *cluster) *appsv1.DaemonSet {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
		c.node("n3", "zone-a"), c.node("n4", "zone-a"))
	return g2Daemon(c, "observability", "logship", "n1", "n2", "n3")
}

// gap3DaemonPlacement: the shipper's pod for n4 cannot be placed: the node
// has no memory left for its request.
func gap3DaemonPlacement() scenario {
	return scenario{
		expect: g2Exp("daemonset-pod-pending-node-full",
			"A DaemonSet's pod for one node stays Pending with "+
				"\"Insufficient memory\" and the DaemonSet is one pod "+
				"short.",
			"node//n4", "notify", "daemonset/observability/logship").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			ds := gap3Daemon(c)
			c.after(5 * time.Minute)
			message := "0/4 nodes are available: 1 Insufficient memory, " +
				"3 node(s) didn't match Pod's node affinity/selector. " +
				"preemption: 0/4 nodes are available: 1 No preemption " +
				"victims found for incoming pod, 3 Preemption is not " +
				"helpful for scheduling."
			setAgentCounts(ds, 4, 3)
			c.update(ds)
			pod := agentPod(c, ds, "n4", pendingUnscheduled(message))
			// The DaemonSet controller pins each pod to its node by name.
			pod.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.
				NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.
				NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchFields: []corev1.NodeSelectorRequirement{{
					Key: "metadata.name", Operator: corev1.NodeSelectorOpIn,
					Values: []string{c.n("n4")}}}}}}}}
			c.create(pod)
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				g2Event(c, pod, "Pod", "FailedScheduling", message,
					"default-scheduler", m)
			}
		},
	}
}

// gap3DaemonPlacementBrief: the pod for the new node waits a minute for
// the node's resources to settle and runs.
func gap3DaemonPlacementBrief() scenario {
	return scenario{
		expect: g2Quiet("daemonset-pod-pending-briefly-quiet",
			"A DaemonSet's pod for a newly added node is Pending for "+
				"under a minute and then runs."),
		build: func(c *cluster) {
			ds := gap3Daemon(c)
			c.after(5 * time.Minute)
			setAgentCounts(ds, 4, 3)
			c.update(ds)
			c.create(agentPod(c, ds, "n4", pendingUnscheduled("0/4 nodes "+
				"are available: 1 node(s) had untolerated taint "+
				"{node.kubernetes.io/not-ready: }.")))
			c.after(40 * time.Second)
			c.update(agentPod(c, ds, "n4", startedNow))
			setAgentCounts(ds, 4, 4)
			c.update(ds)
			c.after(5 * time.Minute)
		},
	}
}

// gap3DaemonFailedLoop: the shipper's pod on n2 is rejected by the
// kubelet with OutOfcpu; the DaemonSet controller deletes it and creates
// it again, and again.
func gap3DaemonFailedLoop() scenario {
	return scenario{
		expect: g2Exp("daemonset-failed-pod-recreated-loop",
			"A DaemonSet's pod on one node is rejected by the kubelet "+
				"every minute and recreated by the controller, which "+
				"reports FailedDaemonPod each time.",
			"node//n2", "notify", "daemonset/observability/logship").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			ds := gap3Daemon(c)
			c.after(5 * time.Minute)
			message := "Pod was rejected: Node didn't have enough " +
				"resource: cpu, requested: 500, used: 3700, capacity: 4000"
			for m := int32(1); m <= 6; m++ {
				pod := agentPod(c, ds, "n2", g2Failed("OutOfcpu", message))
				c.update(pod)
				setAgentCounts(ds, 3, 2)
				c.update(ds)
				g2Event(c, ds, "DaemonSet", "FailedDaemonPod", "Found "+
					"failed daemon pod observability/"+pod.Name+" on node "+
					c.n("n2")+", will try to kill it", "daemonset-controller",
					m)
				c.after(time.Minute)
			}
		},
	}
}

// gap3Budget is a budget that selects label app=value.
func gap3Budget(c *cluster, name, value string) *policyv1.PodDisruptionBudget {
	min := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: clusterMeta(c, "shop", name, "pdb"),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &min,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app": c.n(value)}},
		},
	}
}

// gap3PDBNoPods: a budget's selector has a typo and matches no pod, so a
// drain of n1 evicts both replicas of search at once.
func gap3PDBNoPods() scenario {
	return scenario{
		expect: g2Exp("pdb-selector-matches-no-pods",
			"A budget's selector matches no pod (\"No matching pods "+
				"found\"); a node drain evicts every replica of the "+
				"application at once and it is down until they restart.",
			"poddisruptionbudget/shop/search-pdb", "notify",
			"node//n1", "deployment/shop/search").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "search",
				"registry.example.com/search:2", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			pdb := gap3Budget(c, "search-pdb", "serach")
			c.list(pdb)
			g2Event(c, pdb, "PodDisruptionBudget", "NoPods", "No matching "+
				"pods found", "controllermanager", 1)
			c.after(10 * time.Minute)
			advCordon(c, c.node("n1", "zone-a"))
			c.remove(w.pod(0, "n1"), w.pod(1, "n1"))
			w.setReady(0)
			c.update(w.objects())
			c.after(5 * time.Second)
			c.create(w.pod(2, "n2", startedNow, notReady),
				w.pod(3, "n2", startedNow, notReady))
			c.after(2 * time.Minute)
			c.update(w.pod(2, "n2", startedNow), w.pod(3, "n2", startedNow))
			w.setReady(2)
			c.update(w.objects())
			c.after(5 * time.Minute)
		},
	}
}

// gap3PDBUnmanaged: a budget with a percentage selects bare pods; the
// controller cannot compute the expected count and says so.
func gap3PDBUnmanaged() scenario {
	return scenario{
		expect: g2Exp("pdb-percentage-on-unmanaged-pods",
			"A budget with a percentage selects pods that no controller "+
				"owns; the disruption controller cannot compute the "+
				"expected count.",
			"poddisruptionbudget/shop/queue-pdb", "digest", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "queue",
				"registry.example.com/queue:1", 2)
			for i := range 2 {
				pod := w.pod(i, "n1")
				pod.OwnerReferences = nil
				c.list(pod)
			}
			pdb := gap3Budget(c, "queue-pdb", "queue")
			half := intstr.FromString("50%")
			pdb.Spec.MinAvailable = &half
			c.list(pdb)
			for m := int32(1); m <= 4; m++ {
				g2Event(c, pdb, "PodDisruptionBudget",
					"CalculateExpectedPodCountFailed", "Failed to "+
						"calculate the number of expected pods: found no "+
						"controllers for pod \"queue-a\"",
					"controllermanager", m)
				g2Event(c, pdb, "PodDisruptionBudget", "UnmanagedPods",
					"Pods selected by this PodDisruptionBudget (selector: "+
						"app=queue) were found to be unmanaged. As a "+
						"result, the status of the PDB cannot be "+
						"calculated correctly, which may result in "+
						"undefined behavior.", "controllermanager", m)
				c.after(5 * time.Minute)
			}
		},
	}
}

// gap3FailedJob lists a Job of ns that failed with condition reason, one
// pod failing with message, and returns it.
func gap3FailedJob(
	c *cluster, name, reason, message, podMessage string, exit int32,
) *batchv1.Job {
	c.list(c.node("n1", "zone-a"))
	job := retryJob(c, name, 0)
	c.create(job)
	c.after(30 * time.Second)
	pod := retryPod(c, job, 0, podMessage)
	pod.Spec.NodeName = c.n("n1")
	pod.Status.ContainerStatuses[0].State.Terminated.ExitCode = exit
	c.create(pod)
	failed := job.DeepCopy()
	failed.Status.Active, failed.Status.Failed = 0, 1
	failed.Status.Conditions = []batchv1.JobCondition{{
		Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
		Reason: reason, Message: message,
		LastTransitionTime: metav1.NewTime(c.now),
	}}
	c.update(failed)
	g2Event(c, failed, "Job", reason, message, "job-controller", 1)
	return failed
}

// gap3IndexedJob: an indexed Job fails because three indexes hit their own
// limit on the same input error.
func gap3IndexedJob() scenario {
	return scenario{
		expect: g2Exp("job-max-failed-indexes-exceeded",
			"An indexed Job fails with MaxFailedIndexesExceeded after "+
				"three of its indexes fail with the same input error.",
			"job/batch/import-shards", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			job := gap3FailedJob(c, "import-shards", "MaxFailedIndexesExceeded",
				"Job has exceeded the specified maximal number of failed "+
					"indexes", "invalid record: schema version 9 is not "+
					"supported", 1)
			indexes := "3,7,12"
			job.Status.FailedIndexes = &indexes
			c.update(job)
			c.after(time.Minute)
		},
	}
}

// gap3PodFailurePolicy: a pod exits 42, which the Job's podFailurePolicy
// maps to FailJob; the Job fails after one pod with no retries.
func gap3PodFailurePolicy() scenario {
	return scenario{
		expect: g2Exp("job-pod-failure-policy-fail-job",
			"A Job's pod exits with a code its podFailurePolicy maps to "+
				"FailJob; the Job fails at once without retries.",
			"job/batch/migrate", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			gap3FailedJob(c, "migrate", "PodFailurePolicy", "Container "+
				"app for pod batch/migrate-a failed with exit code 42 "+
				"matching FailJob rule at index 0",
				"configuration error: DATABASE_URL is not set", 42)
			c.after(time.Minute)
		},
	}
}

// gap3TTLFailedJob: a Job with a 30 second TTL fails and is deleted by the
// TTL controller before anyone looks.
func gap3TTLFailedJob() scenario {
	return scenario{
		expect: g2Exp("job-failed-then-ttl-deleted",
			"A Job that failed is deleted 30 seconds later by its "+
				"ttlSecondsAfterFinished; its pod's error was the only "+
				"evidence.",
			"job/batch/rebuild-index", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			job := gap3FailedJob(c, "rebuild-index", "BackoffLimitExceeded",
				"Job has reached the specified backoff limit",
				"FATAL: password authentication failed for user \"indexer\"",
				1)
			c.after(30 * time.Second)
			c.remove(job)
			c.after(time.Minute)
		},
	}
}

// gap3TTLCompletedJob: the same TTL deletes a Job that succeeded.
func gap3TTLCompletedJob() scenario {
	return scenario{
		expect: g2Quiet("job-completed-then-ttl-deleted-quiet",
			"A Job that completed is deleted 30 seconds later by its "+
				"ttlSecondsAfterFinished."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			job := retryJob(c, "rebuild-index", 0)
			c.create(job)
			c.after(30 * time.Second)
			done := job.DeepCopy()
			done.Status.Active, done.Status.Succeeded = 0, 1
			done.Status.Conditions = []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(done)
			c.after(30 * time.Second)
			c.remove(done)
			c.after(5 * time.Minute)
		},
	}
}

// gap3CronTimeZone: a CronJob's timeZone has a trailing space; the
// controller cannot read it and creates no run.
func gap3CronTimeZone() scenario {
	return scenario{
		expect: g2Exp("cronjob-unknown-time-zone",
			"A CronJob's timeZone is not a known zone; the controller "+
				"reports UnknownTimeZone and no run is ever created.",
			"cronjob/reports/nightly-digest", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cron := scheduleCronJob(c, "reports", "nightly-digest",
				"0 2 * * *")
			zone := "Europe/Paris "
			cron.Spec.TimeZone = &zone
			c.list(cron)
			for m := int32(1); m <= 4; m++ {
				c.after(10 * time.Minute)
				g2Event(c, cron, "CronJob", "UnknownTimeZone", "Cannot "+
					"determine if job needs to be started: unknown time "+
					"zone Europe/Paris ", "cronjob-controller", m)
			}
		},
	}
}
