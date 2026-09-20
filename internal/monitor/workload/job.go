package workload

import (
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	batchv1lister "k8s.io/client-go/listers/batch/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectJobIssue returns a finding for a failed or suspended Job.
func DetectJobIssue(job *batchv1.Job) *model.Observation {
	if job == nil {
		return nil
	}
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobFailed:
			if condition.Status != corev1.ConditionTrue {
				continue
			}
			reason := condition.Reason
			if reason == "" {
				reason = constant.ReasonJobFailed
			}
			return observe.Object("job", job, reason)
		case batchv1.JobSuspended:
			if condition.Status == corev1.ConditionTrue {
				return observe.Object(
					"job", job, constant.ReasonJobSuspended,
				)
			}
		}
	}
	return nil
}

// DetectJobExecutionIssue returns deadline and backoff failures.
func DetectJobExecutionIssue(
	job *batchv1.Job,
	now time.Time,
) *model.Observation {
	if job == nil || job.Status.StartTime == nil {
		return nil
	}
	if job.Spec.ActiveDeadlineSeconds != nil && job.Status.Active > 0 &&
		now.Sub(job.Status.StartTime.Time) >= time.Duration(
			*job.Spec.ActiveDeadlineSeconds,
		)*time.Second {
		return observe.Object(
			"job", job, constant.ReasonJobDeadlineExceeded,
		).WithHint(fmt.Sprintf(
			"Job exceeded activeDeadlineSeconds=%d",
			*job.Spec.ActiveDeadlineSeconds,
		))
	}
	if job.Spec.BackoffLimit != nil &&
		job.Status.Failed >= *job.Spec.BackoffLimit &&
		job.Status.CompletionTime == nil {
		return observe.Object(
			"job", job, constant.ReasonJobBackoffLimitExceeded,
		).WithHint(fmt.Sprintf(
			"Job failed %d times; backoffLimit=%d",
			job.Status.Failed,
			*job.Spec.BackoffLimit,
		))
	}
	return nil
}

// JobProcessor is the controller-facing contract for Job processing.
type JobProcessor interface {
	ProcessJob(string, bool) error
}

// JobConfig supplies informer and clock dependencies to a direct Job runtime.
type JobConfig interface {
	configureLister(batchv1lister.JobLister)
}

// JobRuntime owns Job lookup and lifecycle reconciliation while the detectors
// above remain pure workload policy functions.
type JobRuntime struct {
	support runtimeSupport
	lister  batchv1lister.JobLister
}

// NewJobRuntime constructs the direct Job family adapter.
// configureLister supplies the informer-backed Job cache after controller
// setup.
func (r *JobRuntime) configureLister(lister batchv1lister.JobLister) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessJob reconciles one Job queue key.
func (r *JobRuntime) ProcessJob(key string, deleted bool) error {
	return processKey(
		&r.support, key, "job", deleted,
		nil,
		func() (batchv1lister.JobLister, bool) {
			lister := r.support.sourceSnapshot(
				func() batchv1lister.JobLister { return r.lister },
			)
			return lister, lister != nil
		},
		func(
			lister batchv1lister.JobLister,
			namespace, name string,
		) (*batchv1.Job, error) {
			return lister.Jobs(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.support.reconcileGone(subject)
		},
		func(subject model.ObjectRef, job *batchv1.Job) error {
			return r.processJobObject(subject, job)
		},
	)
}

func (r *JobRuntime) processJobObject(
	subject model.ObjectRef,
	job *batchv1.Job,
) error {
	if job == nil || r.support.maintenance(job.Annotations) ||
		JobComplete(job) {
		r.support.reconcileGone(subject)
		return nil
	}
	if obs := DetectJobIssue(job); obs != nil {
		r.support.reconcile(subject, observations(obs))
		return nil
	}
	r.support.reconcile(subject, observations(DetectJobExecutionIssue(
		job,
		r.support.now(),
	)))
	return nil
}

// JobComplete reports whether the Job finished successfully.
func JobComplete(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete &&
			condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
