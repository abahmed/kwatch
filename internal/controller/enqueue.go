package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

func (c *Controller) enqueuePod(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.podQueue.Add(key)
}

func (c *Controller) enqueueNode(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.nodeQueue.Add(key)
}

func (c *Controller) enqueueDeployment(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.deploymentQueue.Add(key)
}

func (c *Controller) enqueueJob(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.jobQueue.Add(key)
}

func (c *Controller) enqueueDaemonSet(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.daemonSetQueue.Add(key)
}

func (c *Controller) enqueueStatefulSet(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.statefulSetQueue.Add(key)
}

func (c *Controller) enqueuePdb(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.pdbQueue.Add(key)
}

func (c *Controller) enqueueCronJob(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.cronJobQueue.Add(key)
}

func (c *Controller) enqueueHorizontalPodAutoscaler(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.hpaQueue.Add(key)
}

func (c *Controller) recordChange(typ kwcontext.ChangeType, resource string, obj interface{}) {
	if c.tracker == nil {
		return
	}
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	ns, name, _ := cache.SplitMetaNamespaceKey(key)
	if name == "" {
		name = key
	}
	c.tracker.Record(kwcontext.Change{
		Resource:  resource,
		Namespace: ns,
		Name:      name,
		Type:      typ,
		Timestamp: time.Now(),
	})
}

func (c *Controller) changeRecordingHandler(resource string, enqueue func(interface{})) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeCreate, resource, obj)
			enqueue(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.recordChange(kwcontext.ChangeUpdate, resource, new)
			enqueue(new)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeDelete, resource, obj)
			enqueue(obj)
		},
	}
}
func (c *Controller) podEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeCreate, "pod", obj)
			if pod, ok := obj.(*corev1.Pod); ok {
				c.addPodToGraph(pod)
			}
			c.enqueuePod(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.recordChange(kwcontext.ChangeUpdate, "pod", new)
			if pod, ok := new.(*corev1.Pod); ok {
				c.removePodFromGraph(pod)
				c.addPodToGraph(pod)
			}
			c.enqueuePod(new)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeDelete, "pod", obj)
			if pod, ok := obj.(*corev1.Pod); ok {
				c.removePodFromGraph(pod)
			}
			c.enqueuePod(obj)
		},
	}
}
func (c *Controller) enqueueService(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.serviceQueue.Add(key)
}

func (c *Controller) enqueueEndpointSlice(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.endpointSliceQueue.Add(key)
}

func (c *Controller) enqueueMwc(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.mwcQueue.Add(key)
}

func (c *Controller) enqueueVwc(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.vwcQueue.Add(key)
}

func (c *Controller) enqueueIngress(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.ingressQueue.Add(key)
}

func (c *Controller) enqueueNetpol(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.netpolQueue.Add(key)
}

func (c *Controller) enqueueCpPod(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.cpPodQueue.Add(key)
}
