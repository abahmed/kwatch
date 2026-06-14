package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

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

func (h *handler) ProcessCronJobObject(cj *batchv1.CronJob, deleted bool) error {
	if cj == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("cronjob", cj.Namespace+"/"+cj.Name)
		return nil
	}

	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		ev := h.eventWithConfig(event.Event{
			Resource:  "cronjob",
			PodName:   cj.Name,
			Namespace: cj.Namespace,
			Reason:    "CronJobSuspended",
			Events:    "",
			Logs:      "",
			Labels:    cj.Labels,
		})
		h.report(ev, cj.Namespace+"/"+cj.Name, nil)
		return nil
	}

	if cj.Status.LastScheduleTime == nil || cj.Status.LastScheduleTime.Time.Before(time.Now().Add(-24*time.Hour)) {
		ev := h.eventWithConfig(event.Event{
			Resource:  "cronjob",
			PodName:   cj.Name,
			Namespace: cj.Namespace,
			Reason:    "CronJobNotScheduled",
			Events:    "",
			Logs:      "",
			Labels:    cj.Labels,
		})
		h.report(ev, cj.Namespace+"/"+cj.Name, nil)
		return nil
	}

	h.correlator.ResolveByResource("cronjob", cj.Namespace+"/"+cj.Name)
	return nil
}
