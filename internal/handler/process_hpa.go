package handler

import (
	"fmt"
	"time"

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
		h.clearFirstMaxed(namespace + "/" + name)
		h.correlator.ResolveByResource("horizontalpodautoscaler", namespace+"/"+name)
		return nil
	}
	hpa, err := h.hpaLister.HorizontalPodAutoscalers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstMaxed(namespace + "/" + name)
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
		h.clearFirstMaxed(hpa.Namespace + "/" + hpa.Name)
		h.correlator.ResolveByResource("horizontalpodautoscaler", hpa.Namespace+"/"+hpa.Name)
		return nil
	}

	key := hpa.Namespace + "/" + hpa.Name
	maxed := hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas &&
		hpa.Status.CurrentReplicas < hpa.Status.DesiredReplicas
	if !maxed {
		h.clearFirstMaxed(key)
		h.correlator.ResolveByResource("horizontalpodautoscaler", key)
		return nil
	}

	first := h.markFirstMaxed(key)
	sustained := time.Duration(h.config.HpaMonitor.SustainedMinutes) * time.Minute
	if sustained > 0 && time.Since(first) < sustained {
		return nil
	}

	ev := event.Event{
		Resource:  "horizontalpodautoscaler",
		PodName:   hpa.Name,
		Namespace: hpa.Namespace,
		Reason:    "HPAMaxedOut",
		Events:    "",
		Logs:      "",
		Labels:    hpa.Labels,
		Hint: fmt.Sprintf("pinned at max=%d (desired=%d current=%d) for %s — raise maxReplicas or investigate load",
			hpa.Spec.MaxReplicas, hpa.Status.DesiredReplicas,
			hpa.Status.CurrentReplicas, time.Since(first).Round(time.Minute)),
	}
	inc, action := h.correlator.Process(ev, key, nil)
	if action != model.ActionSkip {
		h.alertManager.NotifyIncident(inc, action)
	}
	return nil
}

func (h *handler) markFirstMaxed(key string) time.Time {
	h.hpaMu.Lock()
	defer h.hpaMu.Unlock()
	if t, ok := h.firstMaxedHPAs[key]; ok {
		return t
	}
	h.firstMaxedHPAs[key] = time.Now()
	return h.firstMaxedHPAs[key]
}

func (h *handler) clearFirstMaxed(key string) {
	h.hpaMu.Lock()
	defer h.hpaMu.Unlock()
	delete(h.firstMaxedHPAs, key)
}
