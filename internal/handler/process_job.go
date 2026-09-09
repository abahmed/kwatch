package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (h *handler) ProcessJob(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid job key %q: %w", key, err)
	}

	if deleted {
		h.reconcileGone(model.NewObjectRef("job", namespace, name))
		return nil
	}

	job, err := h.listers.Job.Jobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(model.NewObjectRef("job", namespace, name))
			return nil
		}
		return fmt.Errorf(
			"failed to get job %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}

	return h.ProcessJobObject(job, false)
}

// DetectJobIssue returns a Signal if the Job has a failed or suspended
// condition. Used for baseline seeding at startup.
func DetectJobIssue(job *batchv1.Job) *model.Observation {
	if job == nil {
		return nil
	}
	for _, c := range job.Status.Conditions {
		switch c.Type {
		case batchv1.JobFailed:
			if c.Status == corev1.ConditionTrue {
				reason := c.Reason
				if reason == "" {
					reason = constant.ReasonJobFailed
				}
				return observe.Object("job", job, reason)
			}
		case batchv1.JobSuspended:
			if c.Status == corev1.ConditionTrue {
				return observe.Object(
					"job", job, constant.ReasonJobSuspended,
				)
			}
		}
	}
	return nil
}

func DetectJobExecutionIssue(
	job *batchv1.Job, now time.Time,
) *model.Observation {
	if job == nil || job.Status.StartTime == nil {
		return nil
	}
	if job.Spec.ActiveDeadlineSeconds != nil && job.Status.Active > 0 &&
		now.Sub(job.Status.StartTime.Time) >= time.Duration(*job.Spec.ActiveDeadlineSeconds)*time.Second {
		return observe.Object(
			"job", job, constant.ReasonJobDeadlineExceeded,
		).WithHint(fmt.Sprintf(
			"Job exceeded activeDeadlineSeconds=%d",
			*job.Spec.ActiveDeadlineSeconds,
		))
	}
	if job.Spec.BackoffLimit != nil && job.Status.Failed >= *job.Spec.BackoffLimit && job.Status.CompletionTime == nil {
		return observe.Object(
			"job", job, constant.ReasonJobBackoffLimitExceeded,
		).WithHint(fmt.Sprintf(
			"Job failed %d times; backoffLimit=%d",
			job.Status.Failed, *job.Spec.BackoffLimit,
		))
	}
	return nil
}

func (h *handler) ProcessJobObject(job *batchv1.Job, deleted bool) error {
	if job == nil {
		return nil
	}

	subject := model.NewObjectRef("job", job.Namespace, job.Name)
	if deleted || h.inMaintenance(job.Annotations) || jobComplete(job) {
		h.reconcileGone(subject)
		return nil
	}

	// A failing Job speaks for itself: the execution checks below describe
	// how it is failing, which the condition already says.
	if obs := DetectJobIssue(job); obs != nil {
		h.reconcile(subject, findings(obs))
		return nil
	}
	h.reconcile(subject, findings(DetectJobExecutionIssue(job, h.now())))
	return nil
}

// jobComplete reports whether the Job finished successfully, which retires
// every incident about it -- including a JobSuspended one from earlier.
func jobComplete(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
