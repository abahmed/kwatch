package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cronReadsEditedSecret builds the invoice-sync CronJob, which reads
// Secret sync-key when a run starts. bob rotates the Secret; after
// quietFor the run fails. With a run in between that succeeded, the
// Secret was read after the edit and worked.
func cronReadsEditedSecret(c *cluster, succeededBetween bool) {
	secret := configSecret(c, "billing", "sync-key", "token")
	cron := scheduleCronJob(c, "billing", "invoice-sync", "0 2 * * *")
	spec := &cron.Spec.JobTemplate.Spec.Template.Spec
	spec.Containers[0].Env = []corev1.EnvVar{configSecretEnv("SYNC_TOKEN",
		"sync-key", "token")}
	c.list(c.node("n1", "zone-a"), secret, cron)
	c.after(20 * time.Minute)
	change := last(c, secret)
	change.Data["token"] = []byte("rotated")
	editedBy(c, change, "bob")
	c.update(change)
	if succeededBetween {
		c.after(time.Hour)
		ok := finishedJob(c, cron, "invoice-sync-1", c.now, time.Minute)
		c.list(ok)
		done := metav1.NewTime(c.now.Add(time.Minute))
		updated := last(c, cron)
		updated.Status.LastSuccessfulTime = &done
		c.update(updated)
	}
	c.after(3 * time.Hour)
	failedAt := c.now
	job := jobOf(c, cron, "invoice-sync-2", failedAt)
	job.Status.Failed = 4
	job.Status.Conditions = ambiguousJobFailed(failedAt)
	c.list(job)
	c.after(10 * time.Minute)
}

// cronSecretEditedHoursBefore: the run fails hours after the rotation
// that nothing read in between.
func cronSecretEditedHoursBefore() scenario {
	return scenario{
		expect: expectation{
			Name: "cron-secret-edited-hours-before",
			Description: "A CronJob run fails hours after the Secret " +
				"it reads was rotated and no run read it in between.",
			Root: "secret/billing/sync-key", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) { cronReadsEditedSecret(c, false) },
	}
}

// cronSecretEditedThenSucceeded: a run read the rotated Secret and
// worked; the next run fails, so the rotation is not the cause.
func cronSecretEditedThenSucceeded() scenario {
	return scenario{
		expect: expectation{
			Name: "cron-secret-edited-then-succeeded",
			Description: "A CronJob run fails after a run that read " +
				"the rotated Secret succeeded.",
			Root: "cronjob/billing/invoice-sync", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
			MustNotBlame: []string{"secret/billing/sync-key"},
		},
		build: func(c *cluster) { cronReadsEditedSecret(c, true) },
	}
}
