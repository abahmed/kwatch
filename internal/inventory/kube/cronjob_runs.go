package kube

import (
	"time"

	robfig "github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// CronJob run-history attributes.
const (
	// AttrConcurrencyPolicy is spec.concurrencyPolicy (Allow by default).
	AttrConcurrencyPolicy = "concurrency.policy"
	// AttrRunsSinceSuccess counts the scheduled runs from the last
	// successful run (or creation) up to the last scheduled run.
	AttrRunsSinceSuccess = "runs.since.success"
)

// maxCountedRuns bounds the schedule walk; a count this high already
// means the CronJob has not succeeded for a long time.
const maxCountedRuns = 100

func cronJobRuns(cron *batchv1.CronJob, attrs map[string]inventory.Value) {
	policy := cron.Spec.ConcurrencyPolicy
	if policy == "" {
		policy = batchv1.AllowConcurrent
	}
	attrs[AttrConcurrencyPolicy] = inventory.Text(string(policy))
	if runs, ok := runsSinceSuccess(cron); ok {
		attrs[AttrRunsSinceSuccess] = inventory.Number(float64(runs))
	}
}

// runsSinceSuccess counts the schedule times after the last success (or
// creation) up to and including the last scheduled run.
func runsSinceSuccess(cron *batchv1.CronJob) (int, bool) {
	last := cron.Status.LastScheduleTime
	if last == nil {
		return 0, false
	}
	schedule, err := robfig.ParseStandard(cron.Spec.Schedule)
	if err != nil {
		return 0, false
	}
	from := cron.CreationTimestamp.Time
	if success := cron.Status.LastSuccessfulTime; success != nil {
		from = success.Time
	}
	if zone := cron.Spec.TimeZone; zone != nil && *zone != "" {
		if location, err := time.LoadLocation(*zone); err == nil {
			from = from.In(location)
		}
	}
	runs := 0
	for at := schedule.Next(from); !at.After(last.Time) &&
		runs < maxCountedRuns; at = schedule.Next(at) {
		runs++
	}
	return runs, true
}
