package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
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

func (h *handler) ProcessDeploymentObject(deploy *appsv1.Deployment, deleted bool) error {
	if deploy == nil {
		return nil
	}

	if deleted {
		h.correlator.ResolveByResource("deployment", deploy.Namespace+"/"+deploy.Name)
		return nil
	}

	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing &&
			c.Status == corev1.ConditionFalse &&
			c.Reason == "ProgressDeadlineExceeded" {
			ev := h.eventWithConfig(event.Event{
				Resource:  "deployment",
				PodName:   deploy.Name,
				Namespace: deploy.Namespace,
				Reason:    c.Reason,
				Events:    "",
				Logs:      "",
				Labels:    deploy.Labels,
			})
			inc, action := h.correlator.Process(ev, deploy.Namespace+"/"+deploy.Name, nil)
			if action != model.ActionSkip {
				h.alertManager.NotifyIncident(inc, action)
			}
			return nil
		}
	}

	h.correlator.ResolveByResource("deployment", deploy.Namespace+"/"+deploy.Name)
	return nil
}
