package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
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

func (h *handler) ProcessJobObject(job *batchv1.Job, deleted bool) error {
	if job == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("job", job.Namespace+"/"+job.Name)
		return nil
	}

	for _, c := range job.Status.Conditions {
		switch c.Type {
		case batchv1.JobComplete:
			if c.Status == corev1.ConditionTrue {
				h.correlator.ResolveByResource("job", job.Namespace+"/"+job.Name)
				return nil
			}
		case batchv1.JobFailed:
			if c.Status == corev1.ConditionTrue {
				reason := c.Reason
				if reason == "" {
					reason = "JobFailed"
				}
				ev := event.Event{
					Resource:  "job",
					PodName:   job.Name,
					Namespace: job.Namespace,
					Reason:    reason,
					Events:    "",
					Logs:      "",
					Labels:    job.Labels,
				}
				inc, action := h.correlator.Process(ev, job.Namespace+"/"+job.Name, nil)
				if action != model.ActionSkip {
					h.alertManager.NotifyIncident(inc, action)
				}
				return nil
			}
		case batchv1.JobSuspended:
			if c.Status == corev1.ConditionTrue {
				ev := event.Event{
					Resource:  "job",
					PodName:   job.Name,
					Namespace: job.Namespace,
					Reason:    "JobSuspended",
					Events:    "",
					Logs:      "",
					Labels:    job.Labels,
				}
				inc, action := h.correlator.Process(ev, job.Namespace+"/"+job.Name, nil)
				if action != model.ActionSkip {
					h.alertManager.NotifyIncident(inc, action)
				}
				return nil
			}
		}
	}

	return nil
}
