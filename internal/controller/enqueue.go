package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/change"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func (c *Controller) recordChange(typ kwcontext.ChangeType, resource string, obj interface{}) {
	c.recordChangeUpdate(resource, nil, obj, typ)
}

func (c *Controller) recordChangeUpdate(resource string, oldObj, newObj interface{}, typ ...kwcontext.ChangeType) {
	if c.tracker == nil {
		return
	}
	changeType := kwcontext.ChangeUpdate
	if len(typ) > 0 {
		changeType = typ[0]
	}
	obj := newObj
	if obj == nil {
		obj = oldObj
	}
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	ns, name, _ := cache.SplitMetaNamespaceKey(key)
	if name == "" {
		name = key
	}
	record := kwcontext.Change{
		Resource:  resource,
		Namespace: ns,
		Name:      name,
		Type:      changeType,
		Timestamp: c.nowTime(),
	}
	if changeType == kwcontext.ChangeUpdate && oldObj != nil && newObj != nil {
		diff := change.Diff(oldObj, newObj)
		record.Fields, record.BeforeHash, record.AfterHash, record.Additional = diff.Fields, diff.BeforeHash, diff.AfterHash, diff.Additional
	}
	c.tracker.Record(record)
}

func (c *Controller) changeRecordingHandler(resource string, enqueue func(interface{})) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordInformerEvent()
			c.recordChange(kwcontext.ChangeCreate, resource, obj)
			enqueue(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.recordInformerEvent()
			c.recordChangeUpdate(resource, old, new)
			enqueue(new)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordInformerEvent()
			c.recordChange(kwcontext.ChangeDelete, resource, obj)
			enqueue(obj)
		},
	}
}

// watch registers HasSynced and a change-recording event handler for every
// informer feeding the pipeline, and marks its workers to start.
func (c *Controller) watch(p *resourcePipeline, informers ...cache.SharedIndexInformer) {
	for _, inf := range informers {
		c.informers = append(c.informers, inf)
		_ = inf.SetWatchErrorHandler(func(_ *cache.Reflector, err error) { c.recordInformerWatchError(err) })
		p.synced = append(p.synced, inf.HasSynced)
		inf.AddEventHandler(c.changeRecordingHandler(p.trackResource(), p.enqueue))
	}
	p.startWorkers = true
}

// listen hooks up event handlers without touching synced; used when the
// informers' caches are already awaited via lister-support sync tracking.
func (c *Controller) listen(p *resourcePipeline, informers ...cache.SharedIndexInformer) {
	for _, inf := range informers {
		c.informers = append(c.informers, inf)
		_ = inf.SetWatchErrorHandler(func(_ *cache.Reflector, err error) { c.recordInformerWatchError(err) })
		inf.AddEventHandler(c.changeRecordingHandler(p.trackResource(), p.enqueue))
	}
	p.startWorkers = true
}

func (c *Controller) podEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordInformerEvent()
			c.recordChange(kwcontext.ChangeCreate, "pod", obj)
			if pod, ok := obj.(*corev1.Pod); ok {
				c.rebuildPodGraph(pod)
			}
			c.pod.enqueue(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.recordInformerEvent()
			c.recordChangeUpdate("pod", old, new)
			if pod, ok := new.(*corev1.Pod); ok {
				c.rebuildPodGraph(pod)
			}
			c.pod.enqueue(new)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordInformerEvent()
			c.recordChange(kwcontext.ChangeDelete, "pod", obj)
			if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
				if namespace, name, splitErr := cache.SplitMetaNamespaceKey(key); splitErr == nil {
					c.removePodFromGraph(namespace, name)
				}
			}
			c.pod.queue.Add(podDeleteQueueKey(obj))
		},
	}
}

func podDeleteQueueKey(obj interface{}) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return ""
	}
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	} else if tombstone, ok := obj.(*cache.DeletedFinalStateUnknown); ok && tombstone != nil {
		obj = tombstone.Obj
	}
	if accessor, err := meta.Accessor(obj); err == nil && accessor.GetUID() != "" {
		return key + "#" + string(accessor.GetUID())
	}
	return key
}
