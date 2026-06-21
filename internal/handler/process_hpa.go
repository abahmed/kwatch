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

// DetectHPAIssue returns a Signal if the HPA has a scaling error or is
// maxed out. Used for baseline seeding at startup.
func DetectHPAIssue(hpa *autoscalingv2.HorizontalPodAutoscaler) *event.Signal {
	key := hpa.Namespace + "/" + hpa.Name

	for i := range hpa.Status.Conditions {
		c := &hpa.Status.Conditions[i]
		if (c.Type == autoscalingv2.AbleToScale || c.Type == autoscalingv2.ScalingActive) &&
			c.Status == corev1.ConditionFalse {
			return &event.Signal{
				Resource:  "horizontalpodautoscaler",
				Reason:    "HPAScalingError",
				Namespace: hpa.Namespace,
				Owner:     key,
				Labels:    hpa.Labels,
				Hint:      fmt.Sprintf("%s: %s — %s", c.Type, c.Reason, c.Message),
			}
		}
	}

	maxed := hpaHasCondition(hpa, autoscalingv2.ScalingLimited, corev1.ConditionTrue) ||
		(hpa.Spec.MaxReplicas > 0 &&
			hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas &&
			hpa.Status.CurrentReplicas < hpa.Status.DesiredReplicas)
	if maxed {
		return &event.Signal{
			Resource:  "horizontalpodautoscaler",
			Reason:    "HPAMaxedOut",
			Namespace: hpa.Namespace,
			Owner:     key,
			Labels:    hpa.Labels,
			Hint:      fmt.Sprintf("pinned at max=%d (current=%d)", hpa.Spec.MaxReplicas, hpa.Status.CurrentReplicas),
		}
	}

	return nil
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
	if sig := DetectHPAIssue(hpa); sig != nil && sig.Reason == "HPAScalingError" {
		h.signalEvent(sig)
	} else {
		h.correlator.MarkResolved(correlation.BuildKey(hpa.Namespace, key, "HPAScalingError", ""))
	}

	// (2) existing maxed logic — but reason-specific resolve
	maxed := hpaHasCondition(hpa, autoscalingv2.ScalingLimited, corev1.ConditionTrue) ||
		(hpa.Spec.MaxReplicas > 0 &&
			hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas &&
			hpa.Status.CurrentReplicas < hpa.Status.DesiredReplicas)
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
		Resource:  "horizontalpodautoscaler",
		Namespace: hpa.Namespace,
		Reason:    "HPAMaxedOut",
		Owner:     key,
		Labels:    hpa.Labels,
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

func hpaHasCondition(hpa *autoscalingv2.HorizontalPodAutoscaler, condType autoscalingv2.HorizontalPodAutoscalerConditionType, status corev1.ConditionStatus) bool {
	for _, c := range hpa.Status.Conditions {
		if c.Type == condType && c.Status == status {
			return true
		}
	}
	return false
}
