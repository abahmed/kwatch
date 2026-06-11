package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

func (h *handler) ProcessHorizontalPodAutoscaler(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid hpa key %q: %w", key, err)
	}
	if deleted {
		h.correlator.ResolveByResource("horizontalpodautoscaler", namespace+"/"+name)
		return nil
	}
	hpa, err := h.hpaLister.HorizontalPodAutoscalers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("horizontalpodautoscaler", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get hpa %s/%s from cache: %w", namespace, name, err)
	}
	return h.ProcessHorizontalPodAutoscalerObject(hpa, false)
}

func (h *handler) ProcessHorizontalPodAutoscalerObject(hpa *autoscalingv2.HorizontalPodAutoscaler, deleted bool) error {
	if hpa == nil {
		return nil
	}
	if deleted {
		h.correlator.ResolveByResource("horizontalpodautoscaler", hpa.Namespace+"/"+hpa.Name)
		return nil
	}

	if hpa.Spec.MaxReplicas > 0 && hpa.Status.CurrentReplicas >= hpa.Spec.MaxReplicas {
		ev := event.Event{
			Resource:  "horizontalpodautoscaler",
			PodName:   hpa.Name,
			Namespace: hpa.Namespace,
			Reason:    "HPAMaxedOut",
			Events:    "",
			Logs:      "",
			Labels:    hpa.Labels,
		}
		inc, action := h.correlator.Process(ev, hpa.Namespace+"/"+hpa.Name, nil)
		if action != model.ActionSkip {
			h.alertManager.NotifyIncident(inc, action)
		}
		return nil
	}

	h.correlator.ResolveByResource("horizontalpodautoscaler", hpa.Namespace+"/"+hpa.Name)
	return nil
}
