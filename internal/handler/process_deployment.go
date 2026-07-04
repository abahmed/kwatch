package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
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

	deploy, err := h.deployLister.Deployments(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.clearFirstUnavailableDeploy(namespace + "/" + name)
			h.correlator.ResolveByResource("deployment", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get deployment %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessDeploymentObject(deploy, false)
}

// DetectDeploymentIssue returns a Signal if the Deployment has a stuck
// rollout or unavailable replicas. Used for baseline seeding at startup.
func DetectDeploymentIssue(deploy *appsv1.Deployment) *event.Signal {
	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing &&
			c.Status == corev1.ConditionFalse &&
			c.Reason == "ProgressDeadlineExceeded" {
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

// availabilityHintDeploy builds a human-readable summary of deployment availability.
func availabilityHintDeploy(deploy *appsv1.Deployment) string {
	unavailable := deploy.Status.UnavailableReplicas
	desired := deploy.Status.Replicas
	ready := deploy.Status.ReadyReplicas
	updated := deploy.Status.UpdatedReplicas
	return fmt.Sprintf("%d/%d replicas unavailable (ready: %d, updated: %d) — check rollout status and pod events",
		unavailable, desired, ready, updated)
}

func (h *handler) ProcessDeploymentObject(deploy *appsv1.Deployment, deleted bool) error {
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
	// Only alert when the observed generation matches (not mid-rollout metadata sync).
	if deploy.Status.Replicas > 0 && deploy.Status.UnavailableReplicas > 0 &&
		deploy.Status.ObservedGeneration >= deploy.Generation {
		first := h.markFirstUnavailableDeploy(key)

		sustained := time.Duration(h.config.RolloutMonitor.SustainedMinutes) * time.Minute
		if sustained > 0 && h.now().Sub(first) < sustained {
			return nil
		}

		h.signalEvent(&event.Signal{
			Resource:  "deployment",
			Namespace: deploy.Namespace,
			Reason:    "DeploymentUnavailable",
			Owner:     key,
			Labels:    deploy.Labels,
			Hint:      availabilityHintDeploy(deploy),
		})
		return nil
	}

	h.clearFirstUnavailableDeploy(key)
	h.correlator.ResolveByResource("deployment", key)
	return nil
}

func (h *handler) markFirstUnavailableDeploy(key string) time.Time {
	h.deployMu.Lock()
	defer h.deployMu.Unlock()
	if t, ok := h.firstUnavailableDeploy[key]; ok {
		return t
	}
	h.firstUnavailableDeploy[key] = h.now()
	return h.firstUnavailableDeploy[key]
}

func (h *handler) clearFirstUnavailableDeploy(key string) {
	h.deployMu.Lock()
	defer h.deployMu.Unlock()
	delete(h.firstUnavailableDeploy, key)
}
