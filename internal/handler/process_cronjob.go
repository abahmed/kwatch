package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// DefaultCronNotScheduledGrace is the grace period added after the expected
// next fire time before alerting, to account for scheduling lag and clock skew.
const DefaultCronNotScheduledGrace = 5 * time.Minute

func (h *handler) ProcessCronJob(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid cronjob key %q: %w", key, err)
	}

	if deleted {
		h.correlator.ResolveByResource("cronjob", namespace+"/"+name)
		return nil
	}

	cj, err := h.cronJobLister.CronJobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("cronjob", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get cronjob %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessCronJobObject(cj, false)
}

// DetectCronJobIssue returns a Signal if the CronJob has a problem
// (suspended or not scheduled). Used for baseline seeding at startup.
func DetectCronJobIssue(cj *batchv1.CronJob) *event.Signal {
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return &event.Signal{
			Resource:  "cronjob",
			Reason:    "CronJobSuspended",
			Namespace: cj.Namespace,
			Owner:     cj.Namespace + "/" + cj.Name,
			Labels:    cj.Labels,
		}
	}

	nextExpected := NextFireAfter(cj.Spec.Schedule, cj.Status.LastScheduleTime, cj.CreationTimestamp.Time, cj.Spec.TimeZone)
	if nextExpected.IsZero() {
		nextExpected = DefaultNextFire(cj.Status.LastScheduleTime, cj.CreationTimestamp.Time)
	}

	threshold := nextExpected.Add(DefaultCronNotScheduledGrace)

	if time.Now().After(threshold) {
		return &event.Signal{
			Resource:  "cronjob",
			Reason:    "CronJobNotScheduled",
			Namespace: cj.Namespace,
			Owner:     cj.Namespace + "/" + cj.Name,
			Labels:    cj.Labels,
		}
	}

	return nil
}

func (h *handler) ProcessCronJobObject(cj *batchv1.CronJob, deleted bool) error {
	if cj == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("cronjob", cj.Namespace+"/"+cj.Name)
		return nil
	}

	if sig := DetectCronJobIssue(cj); sig != nil {
		h.signalEvent(sig)
		return nil
	}

	h.correlator.ResolveByResource("cronjob", cj.Namespace+"/"+cj.Name)
	return nil
}

// NextFireAfter returns the time the CronJob should have next fired, based on
// its schedule. If LastScheduleTime is nil, it uses CreationTimestamp as the
// reference point. Returns zero time if the schedule cannot be parsed.
// timeZone is the optional IANA timezone from cj.Spec.TimeZone (k8s >=1.27).
func NextFireAfter(schedule string, lastSchedule *metav1.Time, creation time.Time, timeZone *string) time.Time {
	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}
	}
	ref := creation
	if lastSchedule != nil {
		ref = lastSchedule.Time
	}
	if timeZone != nil && *timeZone != "" {
		loc, err := time.LoadLocation(*timeZone)
		if err == nil {
			ref = ref.In(loc)
		}
	}
	return sched.Next(ref)
}

// DefaultNextFire is a fallback when the schedule cannot be parsed. It mimics
// the original 24h heuristic.
func DefaultNextFire(lastSchedule *metav1.Time, creation time.Time) time.Time {
	if lastSchedule == nil {
		return creation.Add(24 * time.Hour)
	}
	return lastSchedule.Time.Add(24 * time.Hour)
}
