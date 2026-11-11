package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// batchNormalScenarios are Jobs and CronJobs in states that are normal
// or over: a run that failed before later runs succeeded, a helper
// container that finished first, one refused pod create that the next
// try got past, and work suspended on purpose.
func batchNormalScenarios() []scenario {
	return []scenario{
		cronRunFailedThenRecovered(), jobHelperContainerFinished(),
		jobCreateRefusedOnce(), cronJobSuspended(), jobSuspended(),
	}
}

// cronRunFailedThenRecovered: one run of a five-minute CronJob fails,
// then every later run succeeds. The failed Job stays in the API (its
// history limit keeps one), but the CronJob recovered: the incident
// ends with the first good run and does not stay open behind it.
func cronRunFailedThenRecovered() scenario {
	return scenario{
		expect: expectation{
			Name: "cron-run-failed-then-recovered",
			Description: "One run of a five-minute CronJob fails; the " +
				"next twelve succeed while the failed Job stays in the " +
				"API.",
			Root: "cronjob/batch/rollup", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(2 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cron := scheduleCronJob(c, "batch", "rollup", "*/5 * * * *")
			cron.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy =
				corev1.RestartPolicyNever
			previous := metav1.NewTime(c.now.Truncate(5 * time.Minute))
			cron.Status.LastScheduleTime = &previous
			cron.Status.LastSuccessfulTime = &previous
			c.list(cron)
			c.now = previous.Add(5*time.Minute + time.Second)
			heldoutFailedRun(c, cron, 0, "panic: rollup table is locked")
			var kept []*batchv1.Job
			for run := 1; run <= 12; run++ {
				c.now = previous.Add(time.Duration(run+1)*5*time.Minute +
					time.Second)
				kept = append(kept, cronGoodRun(c, cron, run))
				if len(kept) > 3 {
					c.remove(last(c, kept[0]))
					kept = kept[1:]
				}
			}
		},
	}
}

// cronGoodRun records one scheduled run that completes: the CronJob
// starts a Job, its pod exits 0 and the CronJob notes the success.
func cronGoodRun(c *cluster, cron *batchv1.CronJob, run int) *batchv1.Job {
	scheduled := metav1.NewTime(c.now)
	job := heldoutJob(c, cron, run)
	edited := last(c, cron)
	edited.Status.LastScheduleTime = &scheduled
	c.update(edited)
	calmRunJob(c, job, "n1")
	done := metav1.NewTime(c.now.Add(-time.Minute))
	edited = last(c, cron)
	edited.Status.LastSuccessfulTime = &done
	c.update(edited)
	return job
}

// jobHelperContainerFinished: a Job pod runs a loader and a helper that
// fetches its input. The helper exits 0 after a minute and the loader
// works for ten more: Kubernetes reports the running pod as not ready
// (a container is not running), which is how such a pod is meant to
// look.
func jobHelperContainerFinished() scenario {
	return scenario{
		expect: expectation{
			Name: "job-helper-container-finished",
			Description: "One container of a two-container Job pod " +
				"exits 0 early; the other keeps working and the pod " +
				"shows Ready=False until the Job completes.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.after(time.Minute)
			job := retryJob(c, "etl", 3)
			job.Spec.Template.Spec.Containers = append(
				job.Spec.Template.Spec.Containers, corev1.Container{
					Name: "fetch", Image: "registry.example.com/fetch:1.2",
				})
			c.create(job)
			pod := helperPod(c, job)
			c.create(pod)
			c.after(time.Minute)
			pod = helperDone(c, pod, 1)
			c.update(pod)
			c.after(10 * time.Minute)
			pod = helperDone(c, pod, 0)
			pod.Status.Phase = corev1.PodSucceeded
			c.update(pod)
			finishJob(c, job)
		},
	}
}

// helperPod is the Job's pod with every container running and ready.
func helperPod(c *cluster, job *batchv1.Job) *corev1.Pod {
	pod := retryPod(c, job, 0, "")
	pod.Spec.NodeName = c.n("n1")
	running(c, pod)
	startedNow(c, pod)
	return pod
}

// helperDone ends container i of the pod with exit 0 and turns the
// pod's readiness off, as the kubelet does.
func helperDone(c *cluster, pod *corev1.Pod, i int) *corev1.Pod {
	out := pod.DeepCopy()
	started := *out.Status.StartTime
	notReady(c, out)
	for j := range out.Status.ContainerStatuses {
		status := &out.Status.ContainerStatuses[j]
		status.Ready = status.State.Running != nil && j != i
	}
	out.Status.ContainerStatuses[i].State = corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{
			ExitCode: 0, Reason: "Completed", StartedAt: started,
			FinishedAt: metav1.NewTime(c.now),
		},
	}
	return out
}

// finishJob marks the Job complete now.
func finishJob(c *cluster, job *batchv1.Job) {
	done := last(c, job)
	now := metav1.NewTime(c.now)
	done.Status.Active, done.Status.Succeeded = 0, 1
	done.Status.CompletionTime = &now
	done.Status.Conditions = []batchv1.JobCondition{{
		Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
		LastTransitionTime: now,
	}}
	c.update(done)
}

// jobCreateRefusedOnce: the Job controller's first pod create is
// refused by an admission webhook that timed out. Ten seconds later the
// create goes through, the pod runs and the Job completes within a
// minute. The refusal is over before anyone could act on it.
func jobCreateRefusedOnce() scenario {
	return scenario{
		expect: expectation{
			Name: "job-create-refused-once",
			Description: "A Job's first pod create is refused once by " +
				"a webhook timeout; the retry creates the pod and the " +
				"Job completes a minute later.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.after(time.Minute)
			job := retryJob(c, "migrate", 3)
			job.Status.Active = 0
			c.create(job)
			c.warn(c.warningEvent(job, "Job", "FailedCreate",
				"Error creating: Internal error occurred: failed "+
					"calling webhook \"pods.policy.example.com\": "+
					"context deadline exceeded", "job-controller", 1))
			c.after(10 * time.Second)
			started := last(c, job)
			started.Status.Active = 1
			c.update(started)
			pod := helperPod(c, job)
			c.create(pod)
			c.after(time.Minute)
			pod = helperDone(c, pod, 0)
			pod.Status.Phase = corev1.PodSucceeded
			c.update(pod)
			finishJob(c, job)
		},
	}
}

// cronJobSuspended: a CronJob is suspended on purpose and stays so. It
// is not failing; nothing is said about it.
func cronJobSuspended() scenario {
	return scenario{
		expect: expectation{
			Name: "cronjob-suspended-on-purpose",
			Description: "A healthy CronJob is suspended and stays " +
				"suspended for an hour.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cron := scheduleCronJob(c, "batch", "rollup", "*/30 * * * *")
			previous := metav1.NewTime(c.now.Truncate(30 * time.Minute))
			cron.Status.LastScheduleTime = &previous
			cron.Status.LastSuccessfulTime = &previous
			c.list(cron)
			c.after(2 * time.Minute)
			paused := last(c, cron)
			paused.Spec.Suspend = boolPtr(true)
			paused.Generation = 2
			c.update(paused)
			c.after(time.Hour)
		},
	}
}

// jobSuspended: a Job is created suspended, as a queue controller
// admits work, and is resumed twenty minutes later and completes.
func jobSuspended() scenario {
	return scenario{
		expect: expectation{
			Name: "job-created-suspended",
			Description: "A Job is created with suspend: true, waits " +
				"twenty minutes for its queue, then runs and completes.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.after(time.Minute)
			job := retryJob(c, "queued", 3)
			job.Spec.Suspend = boolPtr(true)
			job.Status = batchv1.JobStatus{}
			c.create(job)
			c.after(20 * time.Minute)
			resumed := last(c, job)
			resumed.Spec.Suspend = boolPtr(false)
			resumed.Status.Active = 1
			resumed.Status.StartTime = &metav1.Time{Time: c.now}
			c.update(resumed)
			pod := helperPod(c, job)
			c.create(pod)
			c.after(time.Minute)
			pod = helperDone(c, pod, 0)
			pod.Status.Phase = corev1.PodSucceeded
			c.update(pod)
			finishJob(c, job)
		},
	}
}
