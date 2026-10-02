package kubeclient

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// SafeEventHandler contains callback panics at the informer boundary. A bad
// resource callback must not stop the shared informer from delivering later
// updates, and the panic value is intentionally not exposed in diagnostics.
// Whether an add belongs to the initial list is passed through unchanged.
// A delete tombstone (delivered after a watch gap) is unwrapped, so handlers
// always receive the last known object.
func SafeEventHandler(
	component string,
	resource string,
	handler cache.ResourceEventHandler,
) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj interface{}, isInInitialList bool) {
			callHandler(component, resource, func() {
				handler.OnAdd(obj, isInInitialList)
			})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			callHandler(component, resource, func() {
				handler.OnUpdate(oldObj, newObj)
			})
		},
		DeleteFunc: func(obj interface{}) {
			callHandler(component, resource, func() {
				handler.OnDelete(unwrapTombstone(obj))
			})
		},
	}
}

func callHandler(component, resource string, callback func()) {
	defer func() {
		if recover() != nil {
			metrics.DefaultRegistry().InformerHandlerPanics.Add(1)
			klog.ErrorS(nil, "informer event handler panic",
				"component", component, "resource", resource)
		}
	}()
	callback()
}

func unwrapTombstone(obj interface{}) interface{} {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		return tombstone.Obj
	}
	return obj
}
