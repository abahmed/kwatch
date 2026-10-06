package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// redisConnectionLine is the first error line every pod's previous log
// holds; the .NET host prints it before the liveness probe kills it.
const redisConnectionLine = "Unhandled exception. StackExchange.Redis." +
	"RedisConnectionException: It was not possible to connect to the " +
	"redis server(s). Error connecting right now. To allow this " +
	"multiplexer to continue retrying until it's able to connect, use " +
	"abortConnect=false in your connection string or AbortOnConnectFail " +
	"= false; in your configuration."

// redisStormLogsOnly: six .NET Deployments are killed by their liveness
// probe (exit 143) because Redis is down, so the termination message is
// empty. Only their previous logs say why, each with the same first
// error line. The line is the shared cause: one failure-signature
// incident quoting it.
func redisStormLogsOnly() scenario { return redisStormLogs(0) }

// redisStormLogsLate is the same storm, but each crash-log line arrives
// lag after the restart that raised the finding: the finding exists
// first and the error line is added to it later.
func redisStormLogsLate() scenario {
	s := redisStormLogs(30 * time.Second)
	s.expect.Name = "redis-storm-logs-late"
	// The late error line regroups the six incidents into the one
	// failure-signature incident before anything is announced.
	s.expect.MaxMessages = 1
	s.expect.Description += " The log lines arrive 30 seconds after " +
		"each restart."
	return s
}

func redisStormLogs(lag time.Duration) scenario {
	apps := []string{"shop/cart", "shop/session", "shop/search",
		"billing/ledger", "billing/invoicer", "billing/exporter"}
	notBlamed := []string{"node//s1", "node//s2"}
	for _, app := range apps {
		notBlamed = append(notBlamed, "deployment/"+app)
	}
	return scenario{
		expect: expectation{
			Name: "redis-storm-logs-only",
			Description: "Six Deployments are killed by their liveness " +
				"probe with an empty termination message; their " +
				"previous logs all start with one Redis error.",
			Root: "failure-signature//CrashLoop unhandled exception. " +
				"stackexchange.redis.redisconnectionexception: it was " +
				"not po",
			Tier: "notify", MaxMessages: 3, MustNotBlame: notBlamed,
		},
		build: func(c *cluster) {
			c.list(c.node("s1", "zone-a"), c.node("s2", "zone-b"))
			var fleet []*workload
			for _, app := range apps {
				fleet = append(fleet, sharedErrorApp(c, app))
			}
			c.after(3 * time.Minute)
			logsOnlyCrash(c, fleet, lag)
		},
	}
}

// logsOnlyCrash restarts both replicas of every workload with exit 143
// and no termination message, then records the first error line of the
// previous log the crash-log round would have read, lag after the
// restart.
func logsOnlyCrash(c *cluster, fleet []*workload, lag time.Duration) {
	emitLog := func(id inventory.EntityID) {
		c.emit(kube.CrashLogObservation(id, c.now,
			[]string{"info: starting", redisConnectionLine}))
	}
	for restarts := int32(1); restarts <= 5; restarts++ {
		var late []inventory.EntityID
		for _, w := range fleet {
			for replica, node := range []string{"s1", "s2"} {
				pod := w.pod(replica, node, crashLoop(143, "Error", "",
					restarts))
				c.update(pod)
				id := kube.ContainerID(pod.Namespace, pod.Name,
					pod.Spec.Containers[0].Name)
				if lag == 0 {
					emitLog(id)
				} else {
					late = append(late, id)
				}
			}
			w.setReady(0)
			c.update(w.objects())
		}
		c.after(lag)
		for _, id := range late {
			emitLog(id)
		}
		c.after(time.Minute - lag)
	}
}

// logsOnlyScenarios are failures whose cause only the previous logs
// (or the owned Jobs) carry, not the termination message.
func logsOnlyScenarios() []scenario {
	return []scenario{redisStormLogsOnly(), redisStormLogsLate(),
		cronLastRunFailed()}
}

// cronLastRunFailed: the weekly export's run three days ago hit its
// backoff limit and nothing has run since. One failed run is not
// repeated failure, so the incident is the CronJob's last run.
func cronLastRunFailed() scenario {
	return scenario{
		expect: expectation{
			Name: "cronjob-last-run-failed",
			Description: "A weekly CronJob's last run failed three days " +
				"ago (BackoffLimitExceeded) and no run has succeeded " +
				"since.",
			Root: "cronjob/reports/weekly-export", Tier: "digest",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cron := scheduleCronJob(c, "reports", "weekly-export",
				"0 10 * * 6")
			failedAt := c.now.Add(-3 * 24 * time.Hour)
			lastGood := metav1.NewTime(failedAt.Add(-7 * 24 * time.Hour))
			scheduled := metav1.NewTime(failedAt)
			cron.Status.LastSuccessfulTime = &lastGood
			cron.Status.LastScheduleTime = &scheduled
			c.list(cron)
			job := jobOf(c, cron, "weekly-export-1", failedAt)
			job.Status.Failed = 4
			job.Status.Conditions = []batchv1.JobCondition{{
				Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
				Reason:             "BackoffLimitExceeded",
				Message:            "Job has reached the backoff limit",
				LastTransitionTime: metav1.NewTime(failedAt.Add(time.Hour)),
			}}
			c.list(job)
			c.after(10 * time.Minute)
		},
	}
}
