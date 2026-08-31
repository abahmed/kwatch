package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
)

func (h *handler) ProcessHorizontalPodAutoscaler(
	key string,
	deleted bool,
) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid hpa key %q: %w", key, err)
	}
	if deleted {
		h.clearFirstMaxed(namespace + "/" + name)
		h.clearFirstScalingError(namespace + "/" + name)
		h.correlator.ResolveByResource(
			"horizontalpodautoscaler",
			namespace+"/"+name,
		)
		return nil
	}
	hpa, err := h.listers.HPA.HorizontalPodAutoscalers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstMaxed(namespace + "/" + name)
			h.clearFirstScalingError(namespace + "/" + name)
			h.correlator.ResolveByResource(
				"horizontalpodautoscaler",
				namespace+"/"+name,
			)
			return nil
		}
		return fmt.Errorf(
			"failed to get hpa %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}
	return h.ProcessHorizontalPodAutoscalerObject(hpa, false)
}

// hpaAtMax returns true when the HPA is genuinely maxed out (at the upper
// replica bound). k8s sets ScalingLimited=True with reason=TooManyReplicas
// (at max — a real capacity problem) or reason=TooFewReplicas (at min —
// idle/under-utilized, the opposite of "maxed"). Only the upper bound counts.
func hpaAtMax(hpa *autoscalingv2.HorizontalPodAutoscaler) bool {
	if hpa.Spec.MaxReplicas <= 1 {
		return false // min==max==1 etc. — can't scale, not actionable
	}
	hasScalingLimited := false
	for i := range hpa.Status.Conditions {
		c := &hpa.Status.Conditions[i]
		if c.Type == autoscalingv2.ScalingLimited &&
			c.Status == corev1.ConditionTrue {
			hasScalingLimited = true
			if c.Reason == constant.ReasonTooManyReplicas {
				return true
			}
		}
	}
	// If ScalingLimited is not set, fall through to the desired/replicas
	// check (the HPA wants to scale but hasn't set the condition yet).
	if !hasScalingLimited {
		return hpa.Spec.MaxReplicas > 0 &&
			hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas &&
			hpa.Status.CurrentReplicas < hpa.Status.DesiredReplicas
	}
	return false
}

// DetectHPAIssues returns signals for both scaling errors and maxed-out
// conditions. Used for baseline seeding at startup. Returns multiple signals
// so that both conditions are seeded independently.
func DetectHPAIssues(
	hpa *autoscalingv2.HorizontalPodAutoscaler,
) []*event.Signal {
	if hpa == nil {
		return nil
	}
	key := hpa.Namespace + "/" + hpa.Name
	var out []*event.Signal

	for i := range hpa.Status.Conditions {
		c := &hpa.Status.Conditions[i]
		if (c.Type == autoscalingv2.AbleToScale ||
			c.Type == autoscalingv2.ScalingActive) &&
			(c.Status == corev1.ConditionFalse || c.Status == corev1.ConditionUnknown) {
			if c.Reason == constant.ReasonScalingDisabled {
				continue // target intentionally at 0 replicas — not an error
			}
			out = append(out, &event.Signal{
				Resource:  "horizontalpodautoscaler",
				Reason:    constant.ReasonHPAScalingError,
				Namespace: hpa.Namespace,
				Owner:     key,
				Labels:    hpa.Labels,
				Hint: fmt.Sprintf(
					"%s: %s — %s",
					c.Type,
					c.Reason,
					c.Message,
				),
			})
			break
		}
		if c.Type == autoscalingv2.ScalingLimited &&
			c.Status == corev1.ConditionTrue &&
			c.Reason != constant.ReasonTooManyReplicas && c.Reason != "TooFewReplicas" {
			out = append(out, &event.Signal{
				Resource: "horizontalpodautoscaler", Reason: constant.ReasonHPAScalingLimited,
				Namespace: hpa.Namespace, Owner: key, Labels: hpa.Labels,
				Hint: fmt.Sprintf("ScalingLimited: %s — %s", c.Reason, c.Message),
			})
		}
	}

	if hpaAtMax(hpa) {
		out = append(out, &event.Signal{
			Resource:  "horizontalpodautoscaler",
			Reason:    constant.ReasonHPAMaxedOut,
			Namespace: hpa.Namespace,
			Owner:     key,
			Labels:    hpa.Labels,
			Hint: fmt.Sprintf(
				"pinned at max=%d (current=%d)",
				hpa.Spec.MaxReplicas,
				hpa.Status.CurrentReplicas,
			),
		})
	}

	return out
}

func (h *handler) ProcessHorizontalPodAutoscalerObject(
	hpa *autoscalingv2.HorizontalPodAutoscaler,
	deleted bool,
) error {
	if hpa == nil {
		return nil
	}
	if deleted {
		h.clearFirstMaxed(hpa.Namespace + "/" + hpa.Name)
		h.clearFirstScalingError(hpa.Namespace + "/" + hpa.Name)
		h.correlator.ResolveByResource(
			"horizontalpodautoscaler",
			hpa.Namespace+"/"+hpa.Name,
		)
		return nil
	}
	if h.inMaintenance(hpa.Annotations) {
		h.clearFirstMaxed(hpa.Namespace + "/" + hpa.Name)
		h.clearFirstScalingError(hpa.Namespace + "/" + hpa.Name)
		h.correlator.ResolveByResource("horizontalpodautoscaler", hpa.Namespace+"/"+hpa.Name)
		return nil
	}

	key := hpa.Namespace + "/" + hpa.Name

	// (1) scaling-error detection — sustained check to avoid transient noise
	sigs := DetectHPAIssues(hpa)
	hadError := false
	hadLimited := false
	for _, sig := range sigs {
		if sig.Reason == constant.ReasonHPAScalingError {
			first := h.markFirstScalingError(key)
			sustained := time.Duration(
				h.config.HpaMonitor.SustainedMinutes,
			) * time.Minute
			if sustained <= 0 || h.now().Sub(first) >= sustained {
				h.signalEvent(sig)
			}
			hadError = true
		} else if sig.Reason == constant.ReasonHPAScalingLimited {
			h.signalEvent(sig)
			hadLimited = true
		}
	}
	if !hadError {
		h.clearFirstScalingError(key)
		h.correlator.MarkResolved(
			correlation.BuildKey(
				hpa.Namespace,
				key,
				constant.ReasonHPAScalingError,
				"",
			),
		)
	}
	if !hadLimited {
		h.correlator.MarkResolved(correlation.BuildKey(
			hpa.Namespace, key, constant.ReasonHPAScalingLimited, "",
		))
	}

	// (2) maxed detection
	if !hpaAtMax(hpa) {
		h.clearFirstMaxed(key)
		h.correlator.MarkResolved(
			correlation.BuildKey(
				hpa.Namespace,
				key,
				constant.ReasonHPAMaxedOut,
				"",
			),
		)
		return nil
	}

	first := h.markFirstMaxed(key)
	sustained := adaptiveSustained(
		h.config.HpaMonitor.SustainedMinutes,
		h.config.AdaptiveThresholds,
		hpa.Spec.MaxReplicas,
		hpa.Spec.MaxReplicas-hpa.Status.CurrentReplicas,
	)
	if sustained > 0 && h.now().Sub(first) < sustained {
		return nil
	}

	h.signalEvent(&event.Signal{
		Resource:  "horizontalpodautoscaler",
		Namespace: hpa.Namespace,
		Reason:    constant.ReasonHPAMaxedOut,
		Owner:     key,
		Labels:    hpa.Labels,
		Hint: fmt.Sprintf(
			"pinned at max=%d (desired=%d current=%d) for %s — raise "+
				"maxReplicas or investigate load",
			hpa.Spec.MaxReplicas,
			hpa.Status.DesiredReplicas,
			hpa.Status.CurrentReplicas,
			h.now().Sub(first).Round(time.Minute),
		),
	})
	return nil
}

func (h *handler) markFirstMaxed(key string) time.Time {
	return h.fs.maxedHPAs.mark(key, h.now())
}

func (h *handler) clearFirstMaxed(key string) {
	h.fs.maxedHPAs.clear(key)
}

func (h *handler) markFirstScalingError(key string) time.Time {
	return h.fs.scalingErrorHPAs.mark(key, h.now())
}

func (h *handler) clearFirstScalingError(key string) {
	h.fs.scalingErrorHPAs.clear(key)
}
