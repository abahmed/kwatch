package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// jobLongScenarios are scheduled Jobs that run far longer than usual.
func jobLongScenarios() []scenario {
	return []scenario{jobRunningLong(), jobRunningLongUsual()}
}

// jobRunningLong: the nightly invoice export took 8 to 12 minutes on
// each of its last three runs; today's has run for over two hours.
func jobRunningLong() scenario {
	return scenario{
		expect: expectation{
			Name: "job-running-long",
			Description: "A CronJob's Job has run for over two hours " +
				"while its last three runs took 8 to 12 minutes.",
			Root: "cronjob/billing/invoice-export", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildLongJob(c, 2*time.Hour+10*time.Minute) },
	}
}

// jobRunningLongUsual: the same CronJob, but today's run is still
// within a few times the usual, and under half an hour.
func jobRunningLongUsual() scenario {
	return scenario{
		expect: expectation{
			Name: "job-running-usual",
			Description: "A CronJob's Job has run for 20 minutes while " +
				"its last three runs took 8 to 12 minutes.",
			Quiet: true, MaxMessages: 0, Tail: duration(5 * time.Minute),
		},
		build: func(c *cluster) { buildLongJob(c, 20*time.Minute) },
	}
}

func buildLongJob(c *cluster, runs time.Duration) {
	c.list(c.node("n1", "zone-a"))
	cron := scheduleCronJob(c, "billing", "invoice-export", "0 2 * * *")
	c.list(cron)
	for i, took := range []time.Duration{8 * time.Minute,
		12 * time.Minute, 10 * time.Minute} {
		started := c.now.Add(-time.Duration(3-i) * 24 * time.Hour)
		c.list(finishedJob(c, cron, "invoice-export-"+itoa(i), started,
			took))
	}
	job := activeJob(c, cron, "invoice-export-123", c.now)
	c.list(job)
	c.after(runs)
	c.update(job.DeepCopy())
}

// jobOf is a Job of cron that started at started.
func jobOf(c *cluster, cron *batchv1.CronJob, name string, started time.Time,
) *batchv1.Job {
	yes := true
	meta := c.meta(cron.Namespace, name)
	meta.UID = types.UID(meta.Name)
	meta.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "batch/v1", Kind: "CronJob", Name: cron.Name,
		UID: cron.UID, Controller: &yes,
	}}
	meta.CreationTimestamp = metav1.NewTime(started)
	template := *cron.Spec.JobTemplate.Spec.Template.DeepCopy()
	return &batchv1.Job{
		ObjectMeta: meta,
		Spec:       batchv1.JobSpec{Template: template},
		Status:     batchv1.JobStatus{StartTime: &metav1.Time{Time: started}},
	}
}

func activeJob(c *cluster, cron *batchv1.CronJob, name string,
	started time.Time) *batchv1.Job {
	job := jobOf(c, cron, name, started)
	job.Status.Active = 1
	return job
}

func finishedJob(c *cluster, cron *batchv1.CronJob, name string,
	started time.Time, took time.Duration) *batchv1.Job {
	job := jobOf(c, cron, name, started)
	done := metav1.NewTime(started.Add(took))
	job.Status.Succeeded = 1
	job.Status.CompletionTime = &done
	job.Status.Conditions = []batchv1.JobCondition{{
		Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
		LastTransitionTime: done,
	}}
	return job
}
