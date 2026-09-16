package workload

import (
	"time"

	"github.com/robfig/cron/v3"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	"k8s.io/klog/v2"
)

// DefaultCronNotScheduledGrace accounts for scheduling lag and clock skew.
const DefaultCronNotScheduledGrace = 5 * time.Minute

// DetectCronJobIssue returns a finding for a suspended or late CronJob.
func DetectCronJobIssue(
	cj *batchv1.CronJob,
	now time.Time,
) *model.Observation {
	if cj == nil {
		return nil
	}
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return observe.Object(
			"cronjob", cj, constant.ReasonCronJobSuspended,
		)
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
	if cj.Spec.StartingDeadlineSeconds != nil &&
		*cj.Spec.StartingDeadlineSeconds > 0 {
		grace = time.Duration(*cj.Spec.StartingDeadlineSeconds) * time.Second
	}
	if !now.Before(nextExpected.Add(grace)) {
		return observe.Object(
			"cronjob", cj, constant.ReasonCronJobNotScheduled,
		)
	}
	return nil
}

// NextFireAfter returns the next schedule time or zero for an invalid
// schedule.
func NextFireAfter(
	schedule string,
	lastSchedule *metav1.Time,
	creation time.Time,
	timeZone *string,
) time.Time {
	scheduleParser, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}
	}
	ref := creation
	if lastSchedule != nil {
		ref = lastSchedule.Time
	}
	if timeZone != nil && *timeZone != "" {
		location, err := time.LoadLocation(*timeZone)
		if err == nil {
			ref = ref.In(location)
		} else {
			klog.ErrorS(
				err,
				"cronjob has invalid timezone, using UTC",
				"timeZone",
				*timeZone,
			)
		}
	}
	return scheduleParser.Next(ref)
}

// DefaultNextFire supplies the historical fallback for invalid schedules.
func DefaultNextFire(lastSchedule *metav1.Time, creation time.Time) time.Time {
	if lastSchedule == nil {
		return creation.Add(24 * time.Hour)
	}
	return lastSchedule.Time.Add(24 * time.Hour)
}

// CronJobProcessor is the controller-facing contract for CronJobs.
type CronJobProcessor interface {
	ProcessCronJob(string, bool) error
}

// CronJobConfig wires informer and clock dependencies into a direct runtime.
type CronJobConfig interface {
	configureLister(batchv1lister.CronJobLister)
}

// CronJobRuntime owns CronJob lookup and suspension lifecycle policy.
type CronJobRuntime struct {
	support runtimeSupport
	lister  batchv1lister.CronJobLister
	first   firstSeen
}

// NewCronJobRuntime constructs the direct CronJob family adapter.
// configureLister supplies the informer-backed CronJob cache.
func (r *CronJobRuntime) configureLister(
	lister batchv1lister.CronJobLister,
) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessCronJob reconciles one CronJob queue key.
func (r *CronJobRuntime) ProcessCronJob(key string, deleted bool) error {
	return processKey(
		&r.support, key, "cronjob", deleted,
		nil,
		func() (batchv1lister.CronJobLister, bool) {
			lister := r.support.sourceSnapshot(
				func() batchv1lister.CronJobLister { return r.lister },
			)
			return lister, lister != nil
		},
		func(
			lister batchv1lister.CronJobLister,
			namespace, name string,
		) (*batchv1.CronJob, error) {
			return lister.CronJobs(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.clearFirst(subject.Namespace + "/" + subject.Name)
			r.support.reconcileGone(subject)
		},
		func(subject model.ObjectRef, cj *batchv1.CronJob) error {
			return r.processCronJobObject(subject, cj)
		},
	)
}

func (r *CronJobRuntime) processCronJobObject(
	subject model.ObjectRef,
	cj *batchv1.CronJob,
) error {
	if cj == nil {
		return nil
	}
	key := cj.Namespace + "/" + cj.Name
	if r.support.maintenance(cj.Annotations) {
		r.clearFirst(key)
		r.support.reconcileGone(subject)
		return nil
	}
	obs := DetectCronJobIssue(cj, r.support.now())
	if obs != nil && obs.Reason == constant.ReasonCronJobSuspended {
		first := r.first.mark(key, r.support.now())
		sustained := time.Duration(
			r.support.runtime.CronJobMonitor().SustainedMinutes,
		) * time.Minute
		if sustained > 0 && r.support.now().Sub(first) < sustained {
			return nil
		}
		r.support.reconcile(subject, observations(obs))
		return nil
	}
	r.clearFirst(key)
	r.support.reconcile(subject, observations(obs))
	return nil
}

func (r *CronJobRuntime) clearFirst(key string) {
	r.first.clear(key)
}
