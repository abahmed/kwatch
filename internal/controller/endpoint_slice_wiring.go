package controller

import (
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

const endpointSliceServiceLabel = "kubernetes.io/service-name"

// wireEndpointSlices gives Service and admission-webhook monitoring the same
// EndpointSlice cache. The cache is also required for startup baseline checks
// when only admission-webhook monitoring is enabled.
func (c *Controller) wireEndpointSlices(cfg *config.Config, fs factorySet) {
	serviceMonitor := cfg.ServiceMonitor.Enabled
	webhookMonitor := cfg.AdmissionWebhookMonitor.Enabled
	if !serviceMonitor && !webhookMonitor {
		return
	}

	c.endpointSliceLister = fs.endpointSliceLister()
	c.watchWithHandler(
		c.endpointSlice,
		serviceMonitor,
		c.endpointSliceEventHandler(serviceMonitor),
		fs.endpointSliceInformers()...,
	)
}

func (c *Controller) endpointSliceEventHandler(
	serviceMonitor bool,
) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.recordInformerEvent()
			c.recordChange(kwcontext.ChangeCreate, "endpointslice", obj)
			c.handleEndpointSliceEvent(nil, obj, serviceMonitor, false)
		},
		UpdateFunc: func(old, obj interface{}) {
			c.recordInformerEvent()
			c.recordChangeUpdate("endpointslice", old, obj)
			c.handleEndpointSliceEvent(old, obj, serviceMonitor, false)
		},
		DeleteFunc: func(obj interface{}) {
			c.recordInformerEvent()
			c.recordChange(kwcontext.ChangeDelete, "endpointslice", obj)
			c.handleEndpointSliceEvent(obj, nil, serviceMonitor, true)
		},
	}
}

func (c *Controller) handleEndpointSliceEvent(
	oldObj, newObj interface{},
	serviceMonitor, deleted bool,
) {
	keys := endpointSliceServiceKeys(oldObj, newObj)
	if serviceMonitor {
		if deleted {
			if c.service != nil {
				for key := range keys {
					c.service.queue.Add(key)
				}
			}
		} else if newObj != nil {
			// The normal EndpointSlice worker resolves the current Service from
			// the informer cache. Deletions use the label captured from the
			// event because the object is already absent from that cache.
			if c.endpointSlice != nil {
				c.endpointSlice.enqueue(newObj)
			}
			oldKey := endpointSliceServiceKey(oldObj)
			newKey := endpointSliceServiceKey(newObj)
			if c.service != nil && oldKey != "" && oldKey != newKey {
				c.service.queue.Add(oldKey)
			}
		}
	}
	for key := range keys {
		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		if err == nil {
			c.enqueueWebhooksForServiceKey(namespace, name)
		}
	}
}

func endpointSliceServiceKeys(objects ...interface{}) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, obj := range objects {
		if key := endpointSliceServiceKey(obj); key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func endpointSliceServiceKey(obj interface{}) string {
	epSlice := endpointSliceObject(obj)
	if epSlice == nil {
		return ""
	}
	serviceName := epSlice.Labels[endpointSliceServiceLabel]
	if epSlice.Namespace == "" || serviceName == "" {
		return ""
	}
	return epSlice.Namespace + "/" + serviceName
}

func endpointSliceObject(obj interface{}) *discoveryv1.EndpointSlice {
	switch value := obj.(type) {
	case *discoveryv1.EndpointSlice:
		if value == nil {
			return nil
		}
		return value
	case cache.DeletedFinalStateUnknown:
		return endpointSliceObject(value.Obj)
	case *cache.DeletedFinalStateUnknown:
		if value == nil {
			return nil
		}
		return endpointSliceObject(value.Obj)
	default:
		return nil
	}
}
