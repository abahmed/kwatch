package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (h *handler) ProcessHorizontalPodAutoscaler(
	key string,
	deleted bool,
) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid hpa key %q: %w", key, err)
	}
	subject := model.NewObjectRef(
		"horizontalpodautoscaler", namespace, name,
	)
	if deleted {
		h.clearFirstMaxed(namespace + "/" + name)
		h.clearFirstScalingError(namespace + "/" + name)
		h.reconcileGone(subject)
		return nil
	}
	hpa, err := h.listers.HPA.HorizontalPodAutoscalers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstMaxed(namespace + "/" + name)
			h.clearFirstScalingError(namespace + "/" + name)
			h.reconcileGone(subject)
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
) []*model.Observation {
	if hpa == nil {
		return nil
	}
	var out []*model.Observation

	for i := range hpa.Status.Conditions {
		c := &hpa.Status.Conditions[i]
		if (c.Type == autoscalingv2.AbleToScale ||
			c.Type == autoscalingv2.ScalingActive) &&
			(c.Status == corev1.ConditionFalse || c.Status == corev1.ConditionUnknown) {
			if c.Reason == constant.ReasonScalingDisabled {
				continue // target intentionally at 0 replicas — not an error
			}
			out = append(out, observe.Object(
				"horizontalpodautoscaler", hpa,
				constant.ReasonHPAScalingError,
			).WithHint(fmt.Sprintf(
				"%s: %s — %s",
				c.Type,
				c.Reason,
				c.Message,
			)))
			break
		}
		if c.Type == autoscalingv2.ScalingLimited &&
			c.Status == corev1.ConditionTrue &&
			c.Reason != constant.ReasonTooManyReplicas && c.Reason != "TooFewReplicas" {
			out = append(out, observe.Object(
				"horizontalpodautoscaler", hpa,
				constant.ReasonHPAScalingLimited,
			).WithHint(fmt.Sprintf(
				"ScalingLimited: %s — %s", c.Reason, c.Message,
			)))
		}
	}

	if hpaAtMax(hpa) {
		out = append(out, observe.Object(
			"horizontalpodautoscaler", hpa, constant.ReasonHPAMaxedOut,
		).WithHint(fmt.Sprintf(
			"pinned at max=%d (current=%d)",
			hpa.Spec.MaxReplicas,
			hpa.Status.CurrentReplicas,
		)))
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
	subject := model.NewObjectRef(
		"horizontalpodautoscaler", hpa.Namespace, hpa.Name,
	)
	key := hpa.Namespace + "/" + hpa.Name
	if deleted || h.inMaintenance(hpa.Annotations) {
		h.clearFirstMaxed(key)
		h.clearFirstScalingError(key)
		h.reconcileGone(subject)
		return nil
	}

	var current []*model.Observation
	hadError := false
	for _, obs := range DetectHPAIssues(hpa) {
		switch obs.Reason {
		case constant.ReasonHPAScalingError:
			hadError = true
			// Sustained check: a scaling error that clears on the next tick
			// is the autoscaler working, not an incident.
			first := h.markFirstScalingError(key)
			sustained := time.Duration(
				h.config.HpaMonitor.SustainedMinutes,
			) * time.Minute
			if sustained <= 0 || h.now().Sub(first) >= sustained {
				current = append(current, obs)
			}
		case constant.ReasonHPAScalingLimited:
			current = append(current, obs)
		}
	}
	if !hadError {
		h.clearFirstScalingError(key)
	}

	if !hpaAtMax(hpa) {
		h.clearFirstMaxed(key)
		h.reconcile(subject, current)
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
		for _, obs := range current {
			h.observe(obs)
		}
		return nil
	}

	current = append(current, observe.Object(
		"horizontalpodautoscaler", hpa, constant.ReasonHPAMaxedOut,
	).WithHint(fmt.Sprintf(
		"pinned at max=%d (desired=%d current=%d) for %s — raise "+
			"maxReplicas or investigate load",
		hpa.Spec.MaxReplicas,
		hpa.Status.DesiredReplicas,
		hpa.Status.CurrentReplicas,
		h.now().Sub(first).Round(time.Minute),
	)))
	h.reconcile(subject, current)
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
