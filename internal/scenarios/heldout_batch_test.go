package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// heldoutBatchScenarios are held-out failures of scheduled work and
// autoscaling. See heldoutLibrary for the rule they follow.
func heldoutBatchScenarios() []scenario {
	return []scenario{heldoutCronJobFailing()}
}

// heldoutCronJobFailing: the hourly invoice export starts failing after
// a code change in its image: every run's pod exits with a stack trace
// and the Job gives up at its backoff limit, two runs in a row. The
// CronJob is the root.
func heldoutCronJobFailing() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-cronjob-failing",
			Description: "Two consecutive runs of an hourly CronJob fail: " +
				"each Job's pods exit 1 and the Job reaches its backoff " +
				"limit.",
			Root: "cronjob/billing/invoice-export", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: buildHeldoutCronJob,
	}
}

func buildHeldoutCronJob(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	cron := scheduleCronJob(c, "billing", "invoice-export", "0 * * * *")
	cron.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy =
		corev1.RestartPolicyNever
	// The run an hour before the scenario starts was on time and fine.
	previous := metav1.NewTime(c.now.Truncate(time.Hour).Add(-time.Hour))
	cron.Status.LastScheduleTime = &previous
	cron.Status.LastSuccessfulTime = &previous
	c.list(cron)
	trace := "Traceback (most recent call last):\n  File \"/app/export." +
		"py\", line 88, in render\nKeyError: 'currency'"
	for run := range 2 {
		// Each run starts a few seconds after the hour it is due.
		c.now = c.now.Truncate(time.Hour).Add(
			time.Duration(run)*time.Hour + 3*time.Second)
		heldoutFailedRun(c, cron, run, trace)
	}
	c.after(10 * time.Minute)
}

// heldoutFailedRun records one scheduled run: the CronJob starts a Job,
// its pod fails three times (backoffLimit 2), and the Job fails.
func heldoutFailedRun(c *cluster, cron *batchv1.CronJob, run int,
	message string) {
	scheduled := metav1.NewTime(c.now)
	job := heldoutJob(c, cron, run)
	edited := last(c, cron)
	edited.Status.LastScheduleTime = &scheduled
	edited.Status.Active = []corev1.ObjectReference{{
		Kind: "Job", Namespace: job.Namespace, Name: job.Name,
	}}
	c.update(edited)
	c.create(job)
	for attempt := range 3 {
		pod := heldoutJobPod(c, job, attempt, message)
		c.create(pod)
		c.after(40 * time.Second)
		job = job.DeepCopy()
		job.Status.Failed = int32(attempt + 1)
		c.update(job)
	}
	job = job.DeepCopy()
	job.Status.Active = 0
	job.Status.Conditions = []batchv1.JobCondition{{
		Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
		Reason:             "BackoffLimitExceeded",
		Message:            "Job has reached the specified backoff limit",
		LastTransitionTime: metav1.NewTime(c.now),
	}}
	c.update(job)
	c.warn(c.warningEvent(job, "Job", "BackoffLimitExceeded",
		"Job has reached the specified backoff limit", "job-controller", 1))
	edited = last(c, cron)
	edited.Status.Active = nil
	c.update(edited)
}

func heldoutJob(c *cluster, cron *batchv1.CronJob, run int) *batchv1.Job {
	yes := true
	limit := int32(2)
	name := cron.Name + "-2925" + itoa(run+10)
	labels := map[string]string{"job-name": name}
	template := *cron.Spec.JobTemplate.Spec.Template.DeepCopy()
	template.Labels = labels
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cron.Namespace, UID: types.UID(name),
			Labels: labels, CreationTimestamp: metav1.NewTime(c.now),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: "CronJob", Name: cron.Name,
				UID: cron.UID, Controller: &yes,
			}},
		},
		Spec: batchv1.JobSpec{BackoffLimit: &limit, Template: template},
		Status: batchv1.JobStatus{
			Active: 1, StartTime: &metav1.Time{Time: c.now},
		},
	}
}

// heldoutJobPod is one failed attempt of a Job: the container ran for
// half a minute and exited 1.
func heldoutJobPod(c *cluster, job *batchv1.Job, attempt int,
	message string) *corev1.Pod {
	yes := true
	name := job.Name + "-" + string(rune('a'+attempt))
	started := metav1.NewTime(c.now)
	finished := metav1.NewTime(c.now.Add(30 * time.Second))
	container := job.Spec.Template.Spec.Containers[0]
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: job.Namespace, UID: types.UID(name),
			Labels: job.Labels, CreationTimestamp: started,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: "Job", Name: job.Name,
				UID: job.UID, Controller: &yes,
			}},
		},
		Spec: *job.Spec.Template.Spec.DeepCopy(),
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed, StartTime: &started,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: container.Name, Image: container.Image,
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1, Reason: "Error", Message: message,
						StartedAt: started, FinishedAt: finished,
					},
				},
			}},
		},
	}
}
