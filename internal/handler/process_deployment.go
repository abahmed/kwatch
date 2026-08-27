package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/event"
)

func (h *handler) ProcessDeployment(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid deployment key %q: %w", key, err)
	}

	if deleted {
		h.clearFirstUnavailableDeploy(namespace + "/" + name)
		h.correlator.ResolveByResource("deployment", namespace+"/"+name)
		return nil
	}

	deploy, err := h.listers.Deploy.Deployments(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstUnavailableDeploy(namespace + "/" + name)
			h.correlator.ResolveByResource("deployment", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf(
			"failed to get deployment %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}

	return h.ProcessDeploymentObject(deploy, false)
}

// DetectDeploymentIssue returns a Signal if the Deployment has a stuck
// rollout or unavailable replicas. Used for baseline seeding at startup.
func DetectDeploymentIssue(deploy *appsv1.Deployment) *event.Signal {
	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing &&
			c.Status == corev1.ConditionFalse &&
			c.Reason == constant.ReasonProgressDeadlineExceeded {
			return &event.Signal{
				Resource:  "deployment",
				Reason:    c.Reason,
				Namespace: deploy.Namespace,
				Owner:     deploy.Namespace + "/" + deploy.Name,
				Labels:    deploy.Labels,
			}
		}
	}
	return nil
}

// availabilityHintDeploy builds a human-readable summary of deployment
// availability.
func availabilityHintDeploy(deploy *appsv1.Deployment) string {
	unavailable := deploy.Status.UnavailableReplicas
	desired := deploy.Status.Replicas
	ready := deploy.Status.ReadyReplicas
	updated := deploy.Status.UpdatedReplicas
	return fmt.Sprintf(
		"%d/%d replicas unavailable (ready: %d, updated: %d) — check rollout "+
			"status and pod events",
		unavailable,
		desired,
		ready,
		updated,
	)
}

// DetectDeploymentUnavailable returns a Signal when a Deployment has replicas
// that are not available, ignoring mid-rollout metadata sync (stale observed
// generation). Used for baseline seeding at startup.
func DetectDeploymentUnavailable(deploy *appsv1.Deployment) *event.Signal {
	if deploy.Status.Replicas > 0 && deploy.Status.UnavailableReplicas > 0 &&
		deploy.Status.ObservedGeneration >= deploy.Generation {
		return &event.Signal{
			Resource:  "deployment",
			Namespace: deploy.Namespace,
			Reason:    constant.ReasonDeploymentUnavailable,
			Owner:     deploy.Namespace + "/" + deploy.Name,
			Labels:    deploy.Labels,
		}
	}
	return nil
}

func (h *handler) ProcessDeploymentObject(
	deploy *appsv1.Deployment,
	deleted bool,
) error {
	if deploy == nil {
		return nil
	}

	key := deploy.Namespace + "/" + deploy.Name

	if deleted {
		h.clearFirstUnavailableDeploy(key)
		h.correlator.ResolveByResource("deployment", key)
		return nil
	}

	// Existing: ProgressDeadlineExceeded
	if sig := DetectDeploymentIssue(deploy); sig != nil {
		h.clearFirstUnavailableDeploy(key)
		h.signalEvent(sig)
		return nil
	}

	// New: DeploymentUnavailable — replicas exist but are not ready/available.
	// Only alert when the observed generation matches (not mid-rollout metadata
	// sync).
	if sig := DetectDeploymentUnavailable(deploy); sig != nil {
		first := h.markFirstUnavailableDeploy(key)

		sustained := time.Duration(
			h.config.RolloutMonitor.SustainedMinutes,
		) * time.Minute
		if sustained > 0 && h.now().Sub(first) < sustained {
			return nil
		}

		sig.Hint = availabilityHintDeploy(deploy)
		h.signalEvent(sig)
		return nil
	}

	h.clearFirstUnavailableDeploy(key)
	h.correlator.ResolveByResource("deployment", key)
	return nil
}

func (h *handler) markFirstUnavailableDeploy(key string) time.Time {
	return h.fs.unavailableDeploy.mark(key, h.now())
}

func (h *handler) clearFirstUnavailableDeploy(key string) {
	h.fs.unavailableDeploy.clear(key)
}
