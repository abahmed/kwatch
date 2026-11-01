package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cronRunsKeepFailing: three half-hourly runs of a CronJob fail the same
// way. Each run is a new Job, but people fix the CronJob: one incident,
// told when it starts, when its first run gives up and when the runs in a
// row count as repeated failure, and not for each new run.
func cronRunsKeepFailing() scenario {
	return scenario{
		expect: expectation{
			Name: "cron-runs-keep-failing",
			Description: "Three consecutive runs of a half-hourly CronJob " +
				"fail the same way; each run is a new Job.",
			Root: "cronjob/ops/stats-rollup", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n1"},
		},
		build: buildCronRunsKeepFailing,
	}
}

func buildCronRunsKeepFailing(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	cron := scheduleCronJob(c, "ops", "stats-rollup", "*/30 * * * *")
	cron.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy =
		corev1.RestartPolicyNever
	previous := metav1.NewTime(c.now.Truncate(30 * time.Minute))
	cron.Status.LastScheduleTime = &previous
	cron.Status.LastSuccessfulTime = &previous
	c.list(cron)
	start := previous.Add(30 * time.Minute)
	for run := range 3 {
		c.now = start.Add(time.Duration(run)*30*time.Minute + time.Second)
		heldoutFailedRun(c, cron, run, "panic: rollup table is locked")
	}
	c.after(10 * time.Minute)
}
