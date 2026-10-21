package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// jobRetryScenarios are Jobs whose pods fail while the Job controller
// still has retries left. A failed pod is the Job doing its work until
// half of the retries are spent.
func jobRetryScenarios() []scenario {
	return []scenario{jobRetrySucceeds(), jobRetriesAllFail()}
}

// retryJob is a standalone Job in batch with the given backoff limit.
func retryJob(c *cluster, name string, limit int32) *batchv1.Job {
	meta := c.meta("batch", name)
	meta.UID = types.UID(meta.Name)
	meta.CreationTimestamp = metav1.NewTime(c.now)
	labels := map[string]string{"job-name": meta.Name}
	template := podTemplate(labels, "registry.example.com/report:1.0")
	template.Spec.RestartPolicy = corev1.RestartPolicyNever
	return &batchv1.Job{
		ObjectMeta: meta,
		Spec:       batchv1.JobSpec{BackoffLimit: &limit, Template: template},
		Status: batchv1.JobStatus{
			Active: 1, StartTime: &metav1.Time{Time: c.now},
		},
	}
}

// retryPod is attempt n of the Job: running, or failed with a message.
func retryPod(
	c *cluster, job *batchv1.Job, attempt int, failure string,
) *corev1.Pod {
	yes := true
	name := job.Name + "-" + string(rune('a'+attempt))
	started := metav1.NewTime(c.now)
	state := corev1.ContainerState{Running: &corev1.ContainerStateRunning{
		StartedAt: started}}
	phase := corev1.PodRunning
	if failure != "" {
		phase = corev1.PodFailed
		state = corev1.ContainerState{Terminated: &corev1.
			ContainerStateTerminated{
			ExitCode: 1, Reason: "Error", Message: failure,
			StartedAt:  metav1.NewTime(c.now.Add(-30 * time.Second)),
			FinishedAt: started,
		}}
	}
	container := job.Spec.Template.Spec.Containers[0]
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: job.Namespace, UID: types.UID(name),
			Labels: job.Spec.Template.Labels, CreationTimestamp: started,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: "Job", Name: job.Name,
				UID: job.UID, Controller: &yes,
			}},
		},
		Spec: *job.Spec.Template.Spec.DeepCopy(),
		Status: corev1.PodStatus{
			Phase: phase, StartTime: &started,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: container.Name, Image: container.Image,
				State: state,
			}},
		},
	}
}

// jobRetrySucceeds: the first pod of a report Job fails on a dropped
// connection, the Job controller starts another, and it completes.
func jobRetrySucceeds() scenario {
	return scenario{
		expect: expectation{
			Name: "job-retry-succeeds",
			Description: "A Job's first pod fails once; the Job " +
				"controller retries and the second pod completes.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			job := retryJob(c, "report", 6)
			c.list(job)
			c.after(10 * time.Minute)
			c.create(retryPod(c, job, 0, "connection reset by peer"))
			job.Status.Failed = 1
			c.update(job)
			c.after(40 * time.Second)
			second := retryPod(c, job, 1, "")
			c.create(second)
			c.after(time.Minute)
			done := last(c, second)
			done.Status.Phase = corev1.PodSucceeded
			done.Status.ContainerStatuses[0].State = corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					Reason: "Completed", StartedAt: second.CreationTimestamp,
					FinishedAt: metav1.NewTime(c.now),
				}}
			c.update(done)
			job.Status.Active, job.Status.Succeeded = 0, 1
			job.Status.Conditions = []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(job)
		},
	}
}

// jobRetriesAllFail: every attempt fails the same way until the Job
// gives up. The first failure is advice; from the second, half of the
// four retries are spent and the failures count.
func jobRetriesAllFail() scenario {
	return scenario{
		expect: expectation{
			Name: "job-retries-all-fail",
			Description: "A Job's pods fail on every attempt until its " +
				"backoff limit is reached.",
			Root: "job/batch/report", Tier: "notify", MaxMessages: 3,
			Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			job := retryJob(c, "report", 4)
			c.list(job)
			for attempt := 0; attempt < 5; attempt++ {
				c.after(2 * time.Minute)
				c.create(retryPod(c, job, attempt,
					"relation \"users\" does not exist"))
				job.Status.Failed = int32(attempt + 1)
				c.update(job)
			}
			job.Status.Active = 0
			job.Status.Conditions = []batchv1.JobCondition{{
				Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
				Reason:             "BackoffLimitExceeded",
				Message:            "Job has reached the specified backoff limit",
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(job)
		},
	}
}
