package handler

import (
	"fmt"

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
		h.correlator.ResolveByResource("deployment", namespace+"/"+name)
		return nil
	}

	deploy, err := h.deployLister.Deployments(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("deployment", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get deployment %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessDeploymentObject(deploy, false)
}

// DetectDeploymentIssue returns a Signal if the Deployment has a stuck
// rollout. Used for baseline seeding at startup.
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

func (h *handler) ProcessDeploymentObject(deploy *appsv1.Deployment, deleted bool) error {
	if deploy == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("deployment", deploy.Namespace+"/"+deploy.Name)
		return nil
	}

	if sig := DetectDeploymentIssue(deploy); sig != nil {
		h.signalEvent(sig)
		return nil
	}

	h.correlator.ResolveByResource("deployment", deploy.Namespace+"/"+deploy.Name)
	return nil
}
