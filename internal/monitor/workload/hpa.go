package workload

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
)

// HPAAtMax reports whether an HPA is genuinely pinned at its upper bound.
func HPAAtMax(hpa *autoscalingv2.HorizontalPodAutoscaler) bool {
	if hpa == nil || hpa.Spec.MaxReplicas <= 1 {
		return false
	}
	hasScalingLimited := false
	for i := range hpa.Status.Conditions {
		condition := &hpa.Status.Conditions[i]
		if condition.Type != autoscalingv2.ScalingLimited ||
			condition.Status != corev1.ConditionTrue {
			continue
		}
		hasScalingLimited = true
		if condition.Reason == constant.ReasonTooManyReplicas {
			return true
		}
	}
	if hasScalingLimited {
		return false
	}
	return hpa.Spec.MaxReplicas > 0 &&
		hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas &&
		hpa.Status.CurrentReplicas < hpa.Status.DesiredReplicas
}

// DetectHPAIssues returns independent findings for HPA scaling conditions.
func DetectHPAIssues(
	hpa *autoscalingv2.HorizontalPodAutoscaler,
) []*model.Observation {
	if hpa == nil {
		return nil
	}
	var observations []*model.Observation
	for i := range hpa.Status.Conditions {
		condition := &hpa.Status.Conditions[i]
		if isHPAConditionFailure(condition) {
			observations = append(observations, observe.Object(
				"horizontalpodautoscaler", hpa,
				constant.ReasonHPAScalingError,
			).WithHint(fmt.Sprintf(
				"%s: %s — %s",
				condition.Type,
				condition.Reason,
				condition.Message,
			)))
			break
		}
		if condition.Type == autoscalingv2.ScalingLimited &&
			condition.Status == corev1.ConditionTrue &&
			condition.Reason != constant.ReasonTooManyReplicas &&
			condition.Reason != "TooFewReplicas" {
			observations = append(observations, observe.Object(
				"horizontalpodautoscaler", hpa,
				constant.ReasonHPAScalingLimited,
			).WithHint(fmt.Sprintf(
				"ScalingLimited: %s — %s",
				condition.Reason,
				condition.Message,
			)))
		}
	}

	if HPAAtMax(hpa) {
		observations = append(observations, observe.Object(
			"horizontalpodautoscaler", hpa, constant.ReasonHPAMaxedOut,
		).WithHint(fmt.Sprintf(
			"pinned at max=%d (current=%d)",
			hpa.Spec.MaxReplicas,
			hpa.Status.CurrentReplicas,
		)))
	}
	return observations
}

func isHPAConditionFailure(
	condition *autoscalingv2.HorizontalPodAutoscalerCondition,
) bool {
	if condition == nil ||
		(condition.Type != autoscalingv2.AbleToScale &&
			condition.Type != autoscalingv2.ScalingActive) ||
		(condition.Status != corev1.ConditionFalse &&
			condition.Status != corev1.ConditionUnknown) {
		return false
	}
	return condition.Reason != constant.ReasonScalingDisabled
}

// HPAProcessor is the controller-facing contract for HPAs.
type HPAProcessor interface {
	ProcessHorizontalPodAutoscaler(string, bool) error
}

// HPAConfig wires informer and clock dependencies into a direct runtime.
type HPAConfig interface {
	configureLister(autoscalingv2lister.HorizontalPodAutoscalerLister)
}

// HPARuntime owns HPA lookup and sustained scaling policy.
type HPARuntime struct {
	support      runtimeSupport
	lister       autoscalingv2lister.HorizontalPodAutoscalerLister
	maxed        firstSeen
	scalingError firstSeen
}

// NewHPARuntime constructs the direct HPA family adapter.
// configureLister supplies the informer-backed HPA cache.
func (r *HPARuntime) configureLister(
	lister autoscalingv2lister.HorizontalPodAutoscalerLister,
) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessHorizontalPodAutoscaler reconciles one HPA queue key.
func (r *HPARuntime) ProcessHorizontalPodAutoscaler(
	key string,
	deleted bool,
) error {
	return processKey(
		&r.support, key, "horizontalpodautoscaler", deleted,
		nil,
		func() (autoscalingv2lister.HorizontalPodAutoscalerLister, bool) {
			lister := r.support.sourceSnapshot(
				func() autoscalingv2lister.HorizontalPodAutoscalerLister {
					return r.lister
				},
			)
			return lister, lister != nil
		},
		func(
			lister autoscalingv2lister.HorizontalPodAutoscalerLister,
			namespace, name string,
		) (*autoscalingv2.HorizontalPodAutoscaler, error) {
			return lister.HorizontalPodAutoscalers(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.clear(subject.Namespace + "/" + subject.Name)
			r.support.reconcileGone(subject)
		},
		func(
			subject model.ObjectRef,
			hpa *autoscalingv2.HorizontalPodAutoscaler,
		) error {
			return r.processHPAObject(subject, hpa)
		},
	)
}

func (r *HPARuntime) processHPAObject(
	subject model.ObjectRef,
	hpa *autoscalingv2.HorizontalPodAutoscaler,
) error {
	if hpa == nil {
		return nil
	}
	key := hpa.Namespace + "/" + hpa.Name
	if r.support.maintenance(hpa.Annotations) {
		r.clear(key)
		r.support.reconcileGone(subject)
		return nil
	}
	var current []*model.Observation
	hadError := false
	for _, obs := range DetectHPAIssues(hpa) {
		switch obs.Reason {
		case constant.ReasonHPAScalingError:
			hadError = true
			first := r.scalingError.mark(key, r.support.now())
			sustained := time.Duration(
				r.support.runtime.HpaMonitor().SustainedMinutes,
			) * time.Minute
			if sustained <= 0 || r.support.now().Sub(first) >= sustained {
				current = append(current, obs)
			}
		case constant.ReasonHPAScalingLimited:
			current = append(current, obs)
		}
	}
	if !hadError {
		r.scalingError.clear(key)
	}
	if !HPAAtMax(hpa) {
		r.maxed.clear(key)
		r.support.reconcile(subject, current)
		return nil
	}
	first := r.maxed.mark(key, r.support.now())
	sustained := adaptiveSustained(
		r.support.runtime.HpaMonitor().SustainedMinutes,
		r.support.runtime.AdaptiveThresholds(),
		hpa.Spec.MaxReplicas,
		hpa.Spec.MaxReplicas-hpa.Status.CurrentReplicas,
	)
	if sustained > 0 && r.support.now().Sub(first) < sustained {
		for _, obs := range current {
			r.support.observe(obs)
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
		r.support.now().Sub(first).Round(time.Minute),
	)))
	r.support.reconcile(subject, current)
	return nil
}

func (r *HPARuntime) clear(key string) {
	r.maxed.clear(key)
	r.scalingError.clear(key)
}
