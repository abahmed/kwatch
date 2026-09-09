package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
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
		h.clearFirstSuspendedCJ(namespace + "/" + name)
		h.reconcileGone(model.NewObjectRef("cronjob", namespace, name))
		return nil
	}

	cj, err := h.listers.CronJob.CronJobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstSuspendedCJ(namespace + "/" + name)
			h.reconcileGone(model.NewObjectRef("cronjob", namespace, name))
			return nil
		}
		return fmt.Errorf(
			"failed to get cronjob %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}

	return h.ProcessCronJobObject(cj, false)
}

// DetectCronJobIssue returns a Signal if the CronJob has a problem
// (suspended or not scheduled). Used for baseline seeding at startup.
func DetectCronJobIssue(
	cj *batchv1.CronJob, now time.Time,
) *model.Observation {
	if cj == nil {
		return nil
	}
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return observe.Object("cronjob", cj, constant.ReasonCronJobSuspended)
	}

	nextExpected := NextFireAfter(
		cj.Spec.Schedule,
		cj.Status.LastScheduleTime,
		cj.CreationTimestamp.Time,
		cj.Spec.TimeZone,
	)
	if nextExpected.IsZero() {
		nextExpected = DefaultNextFire(
			cj.Status.LastScheduleTime,
			cj.CreationTimestamp.Time,
		)
	}

	grace := DefaultCronNotScheduledGrace
	if cj.Spec.StartingDeadlineSeconds != nil && *cj.Spec.StartingDeadlineSeconds > 0 {
		grace = time.Duration(*cj.Spec.StartingDeadlineSeconds) * time.Second
	}
	threshold := nextExpected.Add(grace)

	if !now.Before(threshold) {
		return observe.Object(
			"cronjob", cj, constant.ReasonCronJobNotScheduled,
		)
	}

	return nil
}

func (h *handler) ProcessCronJobObject(
	cj *batchv1.CronJob,
	deleted bool,
) error {
	if cj == nil {
		return nil
	}

	subject := model.NewObjectRef("cronjob", cj.Namespace, cj.Name)
	key := cj.Namespace + "/" + cj.Name
	if deleted || h.inMaintenance(cj.Annotations) {
		h.clearFirstSuspendedCJ(key)
		h.reconcileGone(subject)
		return nil
	}

	obs := DetectCronJobIssue(cj, h.now())
	if obs != nil && obs.Reason == constant.ReasonCronJobSuspended {
		// Sustained check: avoid noise from intentional suspension during
		// incident response or maintenance windows.
		first := h.markFirstSuspendedCJ(key)
		sustained := time.Duration(
			h.config.CronJobMonitor.SustainedMinutes,
		) * time.Minute
		if sustained > 0 && h.now().Sub(first) < sustained {
			return nil
		}
		h.reconcile(subject, findings(obs))
		return nil
	}

	h.clearFirstSuspendedCJ(key)
	h.reconcile(subject, findings(obs))
	return nil
}

func (h *handler) markFirstSuspendedCJ(key string) time.Time {
	return h.fs.suspendedCJs.mark(key, h.now())
}

func (h *handler) clearFirstSuspendedCJ(key string) {
	h.fs.suspendedCJs.clear(key)
}

// NextFireAfter returns the time the CronJob should have next fired, based on
// its schedule. If LastScheduleTime is nil, it uses CreationTimestamp as the
// reference point. Returns zero time if the schedule cannot be parsed.
// timeZone is the optional IANA timezone from cj.Spec.TimeZone (k8s >=1.27).
func NextFireAfter(
	schedule string,
	lastSchedule *metav1.Time,
	creation time.Time,
	timeZone *string,
) time.Time {
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
		} else {
			klog.ErrorS(
				err,
				"cronjob has invalid timezone, using UTC",
				"timeZone",
				*timeZone,
			)
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
