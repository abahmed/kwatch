package workload

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectDeploymentIssue returns the Deployment's own rollout failure.
func DetectDeploymentIssue(deploy *appsv1.Deployment) *model.Observation {
	if deploy == nil {
		return nil
	}
	for _, condition := range deploy.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing &&
			condition.Status == corev1.ConditionFalse &&
			condition.Reason == constant.ReasonProgressDeadlineExceeded {
			return observe.Object("deployment", deploy, condition.Reason)
		}
	}
	return nil
}

// DetectDeploymentConditions preserves condition types as stable findings.
func DetectDeploymentConditions(
	deploy *appsv1.Deployment,
) []*model.Observation {
	if deploy == nil {
		return nil
	}
	var observations []*model.Observation
	for _, condition := range deploy.Status.Conditions {
		if condition.Status != corev1.ConditionFalse &&
			condition.Status != corev1.ConditionUnknown &&
			!(condition.Type == appsv1.DeploymentReplicaFailure &&
				condition.Status == corev1.ConditionTrue) {
			continue
		}
		reason := deploymentConditionReason(condition.Type)
		if reason == "" ||
			(condition.Type == appsv1.DeploymentProgressing &&
				condition.Reason == constant.ReasonProgressDeadlineExceeded) {
			continue
		}
		hint := condition.Reason
		if condition.Message != "" {
			hint = condition.Reason + ": " + condition.Message
		}
		observations = append(
			observations,
			observe.Object("deployment", deploy, reason).WithHint(hint),
		)
	}
	return observations
}

func deploymentConditionReason(
	conditionType appsv1.DeploymentConditionType,
) string {
	switch conditionType {
	case appsv1.DeploymentProgressing:
		return constant.ReasonDeploymentProgressing
	case appsv1.DeploymentAvailable:
		return constant.ReasonDeploymentAvailable
	case appsv1.DeploymentReplicaFailure:
		return constant.ReasonDeploymentReplicaFailure
	default:
		return ""
	}
}

// AvailabilityHint returns a human-readable deployment availability summary.
func AvailabilityHint(deploy *appsv1.Deployment) string {
	if deploy == nil {
		return ""
	}
	unavailable := deploy.Status.UnavailableReplicas
	desired := DeploymentDesiredReplicas(deploy)
	if unavailable == 0 && desired > deploy.Status.ReadyReplicas {
		unavailable = desired - deploy.Status.ReadyReplicas
	}
	return fmt.Sprintf(
		"%d/%d replicas unavailable (ready: %d, updated: %d) — check rollout "+
			"status and pod events",
		unavailable,
		desired,
		deploy.Status.ReadyReplicas,
		deploy.Status.UpdatedReplicas,
	)
}

// DetectDeploymentUnavailable returns a finding after the observed generation
// has caught up with the desired generation.
func DetectDeploymentUnavailable(
	deploy *appsv1.Deployment,
) *model.Observation {
	if deploy == nil || !DeploymentUnavailable(deploy) ||
		deploy.Status.ObservedGeneration < deploy.Generation {
		return nil
	}
	return observe.Object(
		"deployment", deploy, constant.ReasonDeploymentUnavailable,
	)
}

// DeploymentDesiredReplicas returns the configured or observed replica count.
func DeploymentDesiredReplicas(deploy *appsv1.Deployment) int32 {
	if deploy == nil {
		return 0
	}
	if deploy.Spec.Replicas != nil {
		return *deploy.Spec.Replicas
	}
	return deploy.Status.Replicas
}

// DeploymentUnavailable reports whether a Deployment has missing replicas.
func DeploymentUnavailable(deploy *appsv1.Deployment) bool {
	if deploy == nil {
		return false
	}
	if deploy.Spec.Replicas == nil {
		return deploy.Status.Replicas > 0 &&
			deploy.Status.UnavailableReplicas > 0
	}
	return *deploy.Spec.Replicas > 0 &&
		deploy.Status.ReadyReplicas < *deploy.Spec.Replicas
}

// DeploymentProcessor is the controller-facing contract for Deployments.
type DeploymentProcessor interface {
	ProcessDeployment(string, bool) error
}

// DeploymentConfig wires informer and clock dependencies into a direct
// Deployment processor.
type DeploymentConfig interface {
	configureLister(appsv1lister.DeploymentLister)
}

// DeploymentRuntime owns Deployment lookup and rollout lifecycle policy.
type DeploymentRuntime struct {
	support runtimeSupport
	lister  appsv1lister.DeploymentLister
	first   firstSeen
}

// NewDeploymentRuntime constructs the direct Deployment family adapter.
// configureLister supplies the informer-backed Deployment cache.
func (r *DeploymentRuntime) configureLister(
	lister appsv1lister.DeploymentLister,
) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessDeployment reconciles one Deployment queue key.
func (r *DeploymentRuntime) ProcessDeployment(
	key string,
	deleted bool,
) error {
	return processKey(
		&r.support, key, "deployment", deleted,
		nil,
		func() (appsv1lister.DeploymentLister, bool) {
			lister := r.support.sourceSnapshot(
				func() appsv1lister.DeploymentLister { return r.lister },
			)
			return lister, lister != nil
		},
		func(
			lister appsv1lister.DeploymentLister,
			namespace, name string,
		) (*appsv1.Deployment, error) {
			return lister.Deployments(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.clearFirst(subject.Namespace + "/" + subject.Name)
			r.support.reconcileGone(subject)
		},
		func(subject model.ObjectRef, deploy *appsv1.Deployment) error {
			return r.processDeploymentObject(subject, deploy)
		},
	)
}

func (r *DeploymentRuntime) processDeploymentObject(
	subject model.ObjectRef,
	deploy *appsv1.Deployment,
) error {
	if deploy == nil {
		return nil
	}
	key := deploy.Namespace + "/" + deploy.Name
	if r.support.maintenance(deploy.Annotations) {
		r.clearFirst(key)
		r.support.reconcileGone(subject)
		return nil
	}
	if obs := DetectDeploymentIssue(deploy); obs != nil {
		r.clearFirst(key)
		r.support.reconcile(subject, observations(obs))
		return nil
	}
	current := DetectDeploymentConditions(deploy)
	if obs := DetectDeploymentUnavailable(deploy); obs != nil {
		first := r.first.mark(key, r.support.now())
		sustained := adaptiveSustained(
			r.support.runtime.Monitors().Rollout().SustainedMinutes,
			r.support.runtime.Monitors().AdaptiveThresholds(),
			DeploymentDesiredReplicas(deploy),
			deploy.Status.UnavailableReplicas,
		)
		if sustained > 0 && r.support.now().Sub(first) < sustained {
			for _, condition := range current {
				r.support.observe(condition)
			}
			return nil
		}
		current = append(
			current,
			obs.WithHint(AvailabilityHint(deploy)),
		)
	} else {
		r.clearFirst(key)
	}
	r.support.reconcile(subject, current)
	return nil
}

func (r *DeploymentRuntime) clearFirst(key string) {
	r.first.clear(key)
}
