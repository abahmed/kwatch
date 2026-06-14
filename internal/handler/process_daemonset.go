package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/event"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

func availabilityHint(ds *appsv1.DaemonSet) string {
	unavailable := ds.Status.NumberUnavailable
	desired := ds.Status.DesiredNumberScheduled
	available := ds.Status.NumberAvailable
	return fmt.Sprintf("%d/%d pods unavailable (available: %d) — check node capacity, taints, or image",
		unavailable, desired, available)
}

func (h *handler) ProcessDaemonSet(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid daemonset key %q: %w", key, err)
	}

	if deleted {
		h.correlator.ResolveByResource("daemonset", namespace+"/"+name)
		return nil
	}

	ds, err := h.dsLister.DaemonSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("daemonset", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get daemonset %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessDaemonSetObject(ds, false)
}

func (h *handler) ProcessDaemonSetObject(ds *appsv1.DaemonSet, deleted bool) error {
	if ds == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("daemonset", ds.Namespace+"/"+ds.Name)
		return nil
	}

	if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberUnavailable > 0 {
		ev := h.eventWithConfig(event.Event{
			Resource:  "daemonset",
			PodName:   ds.Name,
			Namespace: ds.Namespace,
			Reason:    "DaemonSetUnavailable",
			Events:    "",
			Logs:      "",
			Labels:    ds.Labels,
			Hint:      availabilityHint(ds),
		})
		h.report(ev, ds.Namespace+"/"+ds.Name, nil)
		return nil
	}

	h.correlator.ResolveByResource("daemonset", ds.Namespace+"/"+ds.Name)
	return nil
}
