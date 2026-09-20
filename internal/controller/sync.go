package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

// sync functions map each pipeline's dequeued key to its handler entry point.

func (c *Controller) syncPod(ctx context.Context, key string) error {
	return c.components.Pod.Processor.ProcessPod(ctx, key, false)
}

func (c *Controller) syncNode(_ context.Context, key string) error {
	return c.components.Node.Processor.ProcessNode(key, false)
}

func (c *Controller) syncDeployment(_ context.Context, key string) error {
	return c.components.Workload.Deployments.ProcessDeployment(key, false)
}

func (c *Controller) syncJob(_ context.Context, key string) error {
	return c.components.Workload.Jobs.ProcessJob(key, false)
}

func (c *Controller) syncDaemonSet(_ context.Context, key string) error {
	return c.components.Workload.DaemonSets.ProcessDaemonSet(key, false)
}

func (c *Controller) syncStatefulSet(_ context.Context, key string) error {
	return c.components.Workload.StatefulSets.ProcessStatefulSet(key, false)
}

func (c *Controller) syncPdb(_ context.Context, key string) error {
	return c.components.Workload.PDBs.ProcessPdb(key, false)
}

func (c *Controller) syncCronJob(_ context.Context, key string) error {
	return c.components.Workload.CronJobs.ProcessCronJob(key, false)
}

func (c *Controller) syncHorizontalPodAutoscaler(_ context.Context, key string) error {
	return c.components.Workload.HPAs.ProcessHorizontalPodAutoscaler(key, false)
}

func (c *Controller) syncService(_ context.Context, key string) error {
	return c.components.Network.Processor.ProcessService(key, false)
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
	serviceName := epSlice.Labels[endpointSliceServiceLabel]
	if serviceName == "" {
		return nil
	}

	return c.components.Network.Processor.ProcessService(
		namespace+"/"+serviceName, false,
	)
}

func (c *Controller) syncMwc(_ context.Context, key string) error {
	return c.components.Security.Processor.
		ProcessMutatingWebhookConfiguration(key, false)
}

func (c *Controller) syncVwc(_ context.Context, key string) error {
	return c.components.Security.Processor.
		ProcessValidatingWebhookConfiguration(key, false)
}

func (c *Controller) syncIngress(_ context.Context, key string) error {
	return c.components.Network.Processor.ProcessIngress(key, false)
}

func (c *Controller) syncNetpol(_ context.Context, key string) error {
	return c.components.Network.Processor.ProcessNetworkPolicy(key, false)
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

	return c.components.Integration.ControlPlane.ProcessControlPlanePod(pod)
}

func (c *Controller) syncResourceQuota(_ context.Context, key string) error {
	return c.components.Cluster.Processor.ProcessResourceQuota(key, false)
}

func (c *Controller) syncLimitRange(_ context.Context, key string) error {
	return c.components.Cluster.Processor.ProcessLimitRange(key, false)
}

func (c *Controller) syncNamespace(_ context.Context, key string) error {
	return c.components.Cluster.Processor.ProcessNamespace(key, false)
}

func (c *Controller) syncLease(_ context.Context, key string) error {
	return c.components.Cluster.Processor.ProcessLease(key, false)
}

func (c *Controller) syncReplicaSet(_ context.Context, key string) error {
	return c.components.Workload.ReplicaSets.ProcessReplicaSet(key, false)
}
