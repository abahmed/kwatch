package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// gap7aScenarios are workload patterns: rollouts that cannot surge, init
// requests, jobs and cron runs that never finish, leader-bound readiness
// and endpoint removal during a rollout.
func gap7aScenarios() []scenario {
	return []scenario{
		gap7aSurgePodUnplaceable(), gap7aInitRequestTooLarge(),
		gap7aJobFailsOnDrains(), gap7aCronReplaceKillsRuns(),
		gap7aLegacySidecarKeepsJobOpen(), gap7aLeaderReadinessDeadlock(),
		gap7aNoPreStopDelay(),
	}
}

// gap7aSurgePodUnplaceable: a three-replica Deployment with required
// anti-affinity per node and maxUnavailable 0 cannot start a surge pod
// during a rollout: every node already holds a replica.
func gap7aSurgePodUnplaceable() scenario {
	return scenario{
		expect: g2Exp("rollout-surge-pod-no-node-free-of-replica",
			"A rollout of a Deployment with required per-node "+
				"anti-affinity and no unavailable pods allowed stalls: "+
				"the surge pod is Pending on \"didn't match pod "+
				"anti-affinity rules\" while every old pod runs. The "+
				"rollout strategy explains it, not the scheduling "+
				"reason alone.",
			"deployment/shop/search", "notify", "node//n1", "node//n2",
			"node//n3").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "search",
				"registry.example.com/search:6", 3)
			configTemplate(w, func(spec *corev1.PodSpec) {
				term := corev1.PodAffinityTerm{
					TopologyKey: corev1.LabelHostname,
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": w.deployment.Name}}}
				anti := &corev1.PodAntiAffinity{}
				anti.RequiredDuringSchedulingIgnoredDuringExecution =
					[]corev1.PodAffinityTerm{term}
				spec.Affinity = &corev1.Affinity{PodAntiAffinity: anti}
			})
			zero, one := intstr.FromInt32(0), intstr.FromInt32(1)
			w.deployment.Spec.Strategy = appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &zero, MaxSurge: &one}}
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n3"))
			c.after(10 * time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/search:7"
			})
			d := w.deployment
			d.Status.Replicas, d.Status.UpdatedReplicas = 4, 1
			d.Status.ReadyReplicas, d.Status.AvailableReplicas = 3, 3
			d.Status.UnavailableReplicas = 1
			c.update(d)
			c.create(rs)
			message := "0/3 nodes are available: 3 node(s) didn't match " +
				"pod anti-affinity rules. preemption: 0/3 nodes are " +
				"available: 3 No preemption victims found for incoming pod."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 8)
		},
	}
}

// gap7aInitRequestTooLarge: a migration init container requests 6 CPUs
// while the app asks for 200m; the scheduler uses the largest, which no
// node has.
func gap7aInitRequestTooLarge() scenario {
	return g2Release(g2Exp("release-init-container-cpu-request-too-large",
		"A release adds an init container that requests more CPU than "+
			"any node has; the new pod is Pending on \"Insufficient "+
			"cpu\" although the app asks for little. The init "+
			"container's request explains it, not the scheduling "+
			"reason alone.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.InitContainers = []corev1.Container{{Name: "migrate",
				Image: "registry.example.com/migrate:3",
				Resources: corev1.ResourceRequirements{Requests: corev1.
					ResourceList{corev1.ResourceCPU: resource.MustParse("6")}}}}
			spec.Containers[0].Resources.Requests = corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("200m")}
		},
		g2Pending("0/2 nodes are available: 2 Insufficient cpu. "+
			"preemption: 0/2 nodes are available: 2 No preemption "+
			"victims found for incoming pod."))
}

// gap7aJobFailsOnDrains: three drains of the node pool evict the Job's
// pod each time; every eviction counts against its backoff limit and the
// Job fails although its code never failed.
func gap7aJobFailsOnDrains() scenario {
	e := g2Exp("job-fails-backoff-limit-from-node-drains",
		"A Job's pod is evicted by three node drains in a row; the "+
			"evictions count as failures and the Job reaches "+
			"BackoffLimitExceeded though its code never failed. The Job "+
			"counting disruptions (no podFailurePolicy that ignores "+
			"them) is the cause to act on; the drains were deliberate.",
		"job/batch/nightly", "notify").budget(3).tail(10 * time.Minute)
	return scenario{expect: e, build: func(c *cluster) {
		c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
			c.node("n3", "zone-a"))
		job := retryJob(c, "nightly", 2)
		c.create(job)
		nodes := []string{"n1", "n2", "n3"}
		for i, name := range nodes {
			pod := readyJobPod(c, retryPod(c, job, i, ""))
			pod.Spec.NodeName = c.n(name)
			c.create(pod)
			c.after(20 * time.Minute)
			advCordon(c, last(c, c.node(name, "zone-a")))
			failed := readyJobPod(c, retryPod(c, job, i, ""))
			failed.Spec.NodeName = c.n(name)
			failed.Status.Phase = corev1.PodFailed
			failed.Status.Conditions = append(failed.Status.Conditions,
				corev1.PodCondition{Type: corev1.DisruptionTarget,
					Status:  corev1.ConditionTrue,
					Reason:  "EvictionByEvictionAPI",
					Message: "Eviction API: evicting"})
			c.update(failed)
			c.after(time.Minute)
		}
		done := job.DeepCopy()
		done.Status.Active, done.Status.Failed = 0, 3
		done.Status.Conditions = []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
			Reason:             "BackoffLimitExceeded",
			Message:            "Job has reached the specified backoff limit",
			LastTransitionTime: metav1.NewTime(c.now)}}
		c.update(done)
		g2Event(c, done, "Job", "BackoffLimitExceeded", "Job has "+
			"reached the specified backoff limit", "job-controller", 1)
		c.after(10 * time.Minute)
	}}
}

// gap7aCronReplaceKillsRuns: a ten-minute task runs every five minutes
// under concurrencyPolicy Replace; each tick deletes the run still in
// progress and no run ever completes.
func gap7aCronReplaceKillsRuns() scenario {
	return scenario{
		expect: g2Exp("cronjob-replace-policy-kills-every-run",
			"A CronJob whose runs take ten minutes is scheduled every "+
				"five under Replace; each tick deletes the run in "+
				"progress and none completes for hours.",
			"cronjob/reports/slow-export", "digest", "node//n1").
			tail(4 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cron := scheduleCronJob(c, "reports", "slow-export",
				"*/5 * * * *")
			replace := batchv1.ReplaceConcurrent
			cron.Spec.ConcurrencyPolicy = replace
			// The schedule fires on time: only the runs never finish.
			fired := metav1.NewTime(c.now)
			cron.Status.LastScheduleTime = &fired
			c.list(cron)
			var previous *batchv1.Job
			for i := range 10 {
				c.after(5 * time.Minute)
				if previous != nil {
					c.remove(previous)
				}
				job := activeJob(c, cron, "slow-export-"+itoa(i+300), c.now)
				c.create(job)
				scheduled := metav1.NewTime(c.now)
				edited := last(c, cron)
				edited.Status.LastScheduleTime = &scheduled
				c.update(edited)
				previous = job
			}
		},
	}
}

// gap7aLegacySidecarKeepsJobOpen: a Job pod's main container exited 0
// three hours ago; its proxy container, declared as an ordinary
// container, runs on, so the Job never completes.
func gap7aLegacySidecarKeepsJobOpen() scenario {
	return scenario{
		expect: g2Exp("job-never-completes-sidecar-keeps-running",
			"A Job pod's main container exited 0 hours ago but a proxy "+
				"container keeps running, so the Job stays active and "+
				"never completes.",
			"job/batch/report-gen", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			job := retryJob(c, "report-gen", 2)
			job.Spec.Template.Spec.Containers = append(
				job.Spec.Template.Spec.Containers, corev1.Container{
					Name: "proxy", Image: "registry.example.com/proxy:1.4"})
			c.create(job)
			pod := retryPod(c, job, 0, "")
			pod.Spec.NodeName = c.n("n1")
			pod.Spec.Containers = job.Spec.Template.Spec.Containers
			started := metav1.NewTime(c.now.Add(-3 * time.Hour))
			done := corev1.ContainerState{Terminated: &corev1.
				ContainerStateTerminated{ExitCode: 0, Reason: "Completed",
				StartedAt: started, FinishedAt: metav1.NewTime(
					c.now.Add(-170 * time.Minute))}}
			running := corev1.ContainerState{Running: &corev1.
				ContainerStateRunning{StartedAt: started}}
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{Name: "report", State: done},
				{Name: "proxy", Ready: true, State: running}}
			c.create(pod)
			for range 4 {
				c.after(30 * time.Minute)
				c.update(pod.DeepCopy(), job.DeepCopy())
			}
		},
	}
}

// gap7aLeaderReadinessDeadlock: a leader-elected worker reports ready only
// while it holds the lease; during a rollout the old pod keeps it and
// the new pod never becomes ready.
func gap7aLeaderReadinessDeadlock() scenario {
	return g2Release(g2Exp("rollout-readiness-needs-leadership-deadlock",
		"A rollout's new pod reports ready only when it holds the "+
			"leader lease, which the old pod keeps until the new one "+
			"is ready; the rollout makes no progress.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			gap5aProbe(spec)
		},
		func(c *cluster, w *workload) {
			g2Unready("Readiness probe failed: HTTP probe failed with "+
				"statuscode: 503")(c, w)
			d := w.deployment.DeepCopy()
			d.Status.Conditions = []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentProgressing, Status: "False",
				Reason:             "ProgressDeadlineExceeded",
				Message:            "ReplicaSet has timed out progressing.",
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(d)
		})
}

// gap7aNoPreStopDelay: a rollout of a backend whose pods exit at once on
// SIGTERM; the caller's connections to the pod being removed are refused
// and the caller restarts once per replaced pod.
func gap7aNoPreStopDelay() scenario {
	return scenario{
		expect: g2Exp("rollout-callers-see-refused-connections",
			"During a healthy rollout of a backend that exits at once "+
				"on SIGTERM, its caller's connections to the pod being "+
				"removed are refused and the caller restarts once per "+
				"replaced pod.",
			"deployment/shop/api", "digest", "deployment/shop/web",
			"node//n1", "node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{{Name: "V",
					Value: "2"}}
			})
			web := c.deployment("shop", "web", "registry.example.com/web:3", 2)
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n2"))
			for i := range 2 {
				c.after(time.Minute)
				c.create(w.pod(i, "n1", startedNow, notReady))
				c.after(20 * time.Second)
				c.update(w.pod(i, "n1", startedNow))
				c.remove(w.pod(0, "n1"))
				c.update(web.pod(i, []string{"n1", "n2"}[i], crashLoop(1,
					"Error", "Get \"http://api.shop.svc:8080/items\": "+
						"read tcp 10.244.1.5:43210->10.96.22.5:8080: "+
						"read: connection reset by peer", 1)))
				web.setReady(1)
				c.update(web.objects())
				c.after(30 * time.Second)
				c.update(web.pod(i, []string{"n1", "n2"}[i],
					gap2Restarted(1)))
				web.setReady(2)
				c.update(web.objects())
			}
			c.after(10 * time.Minute)
		},
	}
}
