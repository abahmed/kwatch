package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

// sync functions map each pipeline's dequeued key to its handler entry point.

func (c *Controller) syncPod(ctx context.Context, key string) error {
	return c.handler.ProcessPod(ctx, key, false)
}

func (c *Controller) syncNode(_ context.Context, key string) error {
	return c.handler.ProcessNode(key, false)
}

func (c *Controller) syncDeployment(_ context.Context, key string) error {
	return c.handler.ProcessDeployment(key, false)
}

func (c *Controller) syncJob(_ context.Context, key string) error {
	return c.handler.ProcessJob(key, false)
}

func (c *Controller) syncDaemonSet(_ context.Context, key string) error {
	return c.handler.ProcessDaemonSet(key, false)
}

func (c *Controller) syncStatefulSet(_ context.Context, key string) error {
	return c.handler.ProcessStatefulSet(key, false)
}

func (c *Controller) syncPdb(_ context.Context, key string) error {
	return c.handler.ProcessPdb(key, false)
}

func (c *Controller) syncCronJob(_ context.Context, key string) error {
	return c.handler.ProcessCronJob(key, false)
}

func (c *Controller) syncHorizontalPodAutoscaler(_ context.Context, key string) error {
	return c.handler.ProcessHorizontalPodAutoscaler(key, false)
}

func (c *Controller) syncService(_ context.Context, key string) error {
	return c.handler.ProcessService(key, false)
}

func (c *Controller) syncEndpointSlice(_ context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// EndpointSlice names are "<service-name>-<hash>"; resolve the owning
	// Service via the kubernetes.io/service-name label, not the slice name.
	epSlice, err := c.endpointSliceLister.EndpointSlices(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	serviceName := epSlice.Labels["kubernetes.io/service-name"]
	if serviceName == "" {
		return nil
	}

	return c.handler.ProcessService(namespace+"/"+serviceName, false)
}

func (c *Controller) syncMwc(_ context.Context, key string) error {
	return c.handler.ProcessMutatingWebhookConfiguration(key, false)
}

func (c *Controller) syncVwc(_ context.Context, key string) error {
	return c.handler.ProcessValidatingWebhookConfiguration(key, false)
}

func (c *Controller) syncIngress(_ context.Context, key string) error {
	return c.handler.ProcessIngress(key, false)
}

func (c *Controller) syncNetpol(_ context.Context, key string) error {
	return c.handler.ProcessNetworkPolicy(key, false)
}

func (c *Controller) syncCpPod(_ context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pod, err := c.cpPodLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.handler.ProcessControlPlanePod(pod)
}
