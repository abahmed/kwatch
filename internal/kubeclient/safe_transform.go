package kubeclient

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// transformLogEvery limits the log of a transform that keeps panicking:
// the first panic of a resource is logged, then every hundredth.
const transformLogEvery = 100

// SafeTransform contains a panic of an informer transform. The reflector
// runs the transform on its own goroutine, where nothing recovers, so one
// malformed object would end the whole process. The panic is counted like
// a handler panic (the metric and the per-resource ledger health shows)
// and logged without its value, and stub(obj) replaces the object. The
// stub must hold no part of obj: the transform may have stopped halfway
// through trimming a Secret. An empty resource names the panic after the
// object's Go type.
func SafeTransform(
	component, resource string, transform cache.TransformFunc,
	stub func(obj any) any,
) cache.TransformFunc {
	return func(obj any) (out any, err error) {
		defer func() {
			if recover() == nil {
				return
			}
			name := resource
			if name == "" {
				name = fmt.Sprintf("%T", obj)
			}
			count := recordHandlerPanic(component, name)
			if count == 1 || count%transformLogEvery == 0 {
				klog.ErrorS(nil, "informer transform panic; "+
					"the object is replaced by a stub",
					"component", component, "resource", name,
					"object", objectName(obj), "panics", count)
			}
			out, err = stub(obj), nil
		}()
		return transform(obj)
	}
}

// objectName is the namespace and name of obj, for the log only.
func objectName(obj any) string {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return ""
	}
	if accessor.GetNamespace() == "" {
		return accessor.GetName()
	}
	return accessor.GetNamespace() + "/" + accessor.GetName()
}
