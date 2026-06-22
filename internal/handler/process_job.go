package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/event"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
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

	job, err := h.jobLister.Jobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("job", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get job %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessJobObject(job, false)
}

// DetectJobIssue returns a Signal if the Job has a failed or suspended
// condition. Used for baseline seeding at startup.
func DetectJobIssue(job *batchv1.Job) *event.Signal {
	for _, c := range job.Status.Conditions {
		switch c.Type {
		case batchv1.JobFailed:
			if c.Status == corev1.ConditionTrue {
				reason := c.Reason
				if reason == "" {
					reason = "JobFailed"
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
					Reason:    "JobSuspended",
					Namespace: job.Namespace,
					Owner:     job.Namespace + "/" + job.Name,
					Labels:    job.Labels,
				}
			}
		}
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

	// No active failing or suspended condition → ensure any prior Job
	// incident (including JobSuspended) resolves.
	h.correlator.ResolveByResource("job", job.Namespace+"/"+job.Name)
	return nil
}
