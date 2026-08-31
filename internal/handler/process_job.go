package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/event"
)

func (h *handler) ProcessJob(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid job key %q: %w", key, err)
	}

	if deleted {
		h.correlator.ResolveByResource("job", namespace+"/"+name)
		return nil
	}

	job, err := h.listers.Job.Jobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("job", namespace+"/"+name)
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
func DetectJobIssue(job *batchv1.Job) *event.Signal {
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
				return &event.Signal{
					Resource:  "job",
					Reason:    reason,
					Namespace: job.Namespace,
					Owner:     job.Namespace + "/" + job.Name,
					Labels:    job.Labels,
				}
			}
		case batchv1.JobSuspended:
			if c.Status == corev1.ConditionTrue {
				return &event.Signal{
					Resource:  "job",
					Reason:    constant.ReasonJobSuspended,
					Namespace: job.Namespace,
					Owner:     job.Namespace + "/" + job.Name,
					Labels:    job.Labels,
				}
			}
		}
	}
	return nil
}

func DetectJobExecutionIssue(job *batchv1.Job, now time.Time) *event.Signal {
	if job == nil || job.Status.StartTime == nil {
		return nil
	}
	owner := job.Namespace + "/" + job.Name
	if job.Spec.ActiveDeadlineSeconds != nil && job.Status.Active > 0 &&
		now.Sub(job.Status.StartTime.Time) >= time.Duration(*job.Spec.ActiveDeadlineSeconds)*time.Second {
		return &event.Signal{Resource: "job", Namespace: job.Namespace, Owner: owner,
			Reason: constant.ReasonJobDeadlineExceeded, Labels: job.Labels,
			Hint: fmt.Sprintf("Job exceeded activeDeadlineSeconds=%d", *job.Spec.ActiveDeadlineSeconds)}
	}
	if job.Spec.BackoffLimit != nil && job.Status.Failed >= *job.Spec.BackoffLimit && job.Status.CompletionTime == nil {
		return &event.Signal{Resource: "job", Namespace: job.Namespace, Owner: owner,
			Reason: constant.ReasonJobBackoffLimitExceeded, Labels: job.Labels,
			Hint: fmt.Sprintf("Job failed %d times; backoffLimit=%d", job.Status.Failed, *job.Spec.BackoffLimit)}
	}
	return nil
}

func (h *handler) ProcessJobObject(job *batchv1.Job, deleted bool) error {
	if job == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("job", job.Namespace+"/"+job.Name)
		return nil
	}
	if h.inMaintenance(job.Annotations) {
		h.correlator.ResolveByResource("job", job.Namespace+"/"+job.Name)
		return nil
	}

	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			h.correlator.ResolveByResource("job", job.Namespace+"/"+job.Name)
			return nil
		}
	}

	if sig := DetectJobIssue(job); sig != nil {
		h.signalEvent(sig)
		return nil
	}
	if sig := DetectJobExecutionIssue(job, h.now()); sig != nil {
		h.signalEvent(sig)
		return nil
	}

	// No active failing or suspended condition → ensure any prior Job
	// incident (including JobSuspended) resolves.
	h.correlator.ResolveByResource("job", job.Namespace+"/"+job.Name)
	return nil
}
