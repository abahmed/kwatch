package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
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

	// (1) scaling-error detection — independent of maxed
	var scalingErr *autoscalingv2.HorizontalPodAutoscalerCondition
	for i := range hpa.Status.Conditions {
		c := &hpa.Status.Conditions[i]
		if (c.Type == autoscalingv2.AbleToScale || c.Type == autoscalingv2.ScalingActive) &&
			c.Status == corev1.ConditionFalse {
			scalingErr = c
			break
		}
	}
	if scalingErr != nil {
		h.signalEvent(&event.Signal{
			Resource:  "horizontalpodautoscaler",
			PodName:   hpa.Name,
			Namespace: hpa.Namespace,
			Reason:    "HPAScalingError",
			Owner:     key,
			Labels:    hpa.Labels,
			Hint:      fmt.Sprintf("%s: %s — %s", scalingErr.Type, scalingErr.Reason, scalingErr.Message),
		})
	} else {
		h.correlator.MarkResolved(correlation.BuildKey(hpa.Namespace, key, "HPAScalingError", ""))
	}

	// (2) existing maxed logic — but reason-specific resolve
	maxed := hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas &&
		hpa.Status.CurrentReplicas < hpa.Status.DesiredReplicas
	if !maxed {
		h.clearFirstMaxed(key)
		h.correlator.MarkResolved(correlation.BuildKey(hpa.Namespace, key, "HPAMaxedOut", ""))
		return nil
	}

	first := h.markFirstMaxed(key)
	sustained := time.Duration(h.config.HpaMonitor.SustainedMinutes) * time.Minute
	if sustained > 0 && h.now().Sub(first) < sustained {
		return nil
	}

	h.signalEvent(&event.Signal{
		Resource: "horizontalpodautoscaler",
		PodName:  hpa.Name,
		Namespace: hpa.Namespace,
		Reason:   "HPAMaxedOut",
		Owner:    key,
		Labels:   hpa.Labels,
		Hint: fmt.Sprintf("pinned at max=%d (desired=%d current=%d) for %s — raise maxReplicas or investigate load",
			hpa.Spec.MaxReplicas, hpa.Status.DesiredReplicas,
			hpa.Status.CurrentReplicas, h.now().Sub(first).Round(time.Minute)),
	})
	return nil
}

func (h *handler) markFirstMaxed(key string) time.Time {
	h.hpaMu.Lock()
	defer h.hpaMu.Unlock()
	if t, ok := h.firstMaxedHPAs[key]; ok {
		return t
	}
	h.firstMaxedHPAs[key] = h.now()
	return h.firstMaxedHPAs[key]
}

func (h *handler) clearFirstMaxed(key string) {
	h.hpaMu.Lock()
	defer h.hpaMu.Unlock()
	delete(h.firstMaxedHPAs, key)
}
