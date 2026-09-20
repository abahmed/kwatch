package k8s

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// SafeEventHandler contains callback panics at the informer boundary. A bad
// resource callback must not stop the shared informer from delivering later
// updates, and the panic value is intentionally not exposed in diagnostics.
func SafeEventHandler(
	component string,
	resource string,
	handler cache.ResourceEventHandler,
) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			callHandler(component, resource, func() {
				handler.OnAdd(obj, false)
			})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			callHandler(component, resource, func() {
				handler.OnUpdate(oldObj, newObj)
			})
		},
		DeleteFunc: func(obj interface{}) {
			callHandler(component, resource, func() {
				handler.OnDelete(obj)
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
