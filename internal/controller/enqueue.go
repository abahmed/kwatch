package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

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

// watch registers HasSynced and a change-recording event handler for every
// informer feeding the pipeline, and marks its workers to start.
func (c *Controller) watch(p *resourcePipeline, informers ...cache.SharedIndexInformer) {
	for _, inf := range informers {
		p.synced = append(p.synced, inf.HasSynced)
		inf.AddEventHandler(c.changeRecordingHandler(p.trackResource(), p.enqueue))
	}
	p.startWorkers = true
}

// listen hooks up event handlers without touching synced; used when the
// informers' caches are already awaited via lister-support sync tracking.
func (c *Controller) listen(p *resourcePipeline, informers ...cache.SharedIndexInformer) {
	for _, inf := range informers {
		inf.AddEventHandler(c.changeRecordingHandler(p.trackResource(), p.enqueue))
	}
	p.startWorkers = true
}

func (c *Controller) podEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeCreate, "pod", obj)
			if pod, ok := obj.(*corev1.Pod); ok {
				c.rebuildPodGraph(pod)
			}
			c.pod.enqueue(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.recordChange(kwcontext.ChangeUpdate, "pod", new)
			if pod, ok := new.(*corev1.Pod); ok {
				c.rebuildPodGraph(pod)
			}
			c.pod.enqueue(new)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordChange(kwcontext.ChangeDelete, "pod", obj)
			if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
				if namespace, name, splitErr := cache.SplitMetaNamespaceKey(key); splitErr == nil {
					c.removePodFromGraph(namespace, name)
				}
			}
			c.pod.enqueue(obj)
		},
	}
}
