package kube

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// cappedStub stands in the informer cache for an object the object caps
// refused. It keeps only what the cache needs to key the object and to
// notice its next change, so a kind far over its cap costs a few bytes
// per extra object instead of the whole object.
type cappedStub struct {
	metav1.PartialObjectMetadata
}

// DeepCopyObject keeps the stub type, so a copy is still recognised.
func (s *cappedStub) DeepCopyObject() runtime.Object {
	return &cappedStub{PartialObjectMetadata: *s.DeepCopy()}
}

// newCappedStub returns the stub for obj, or obj itself when it has no
// object metadata.
func newCappedStub(obj any) any {
	m, err := meta.Accessor(obj)
	if err != nil {
		return obj
	}
	return &cappedStub{PartialObjectMetadata: metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.GetName(), Namespace: m.GetNamespace(),
			UID: m.GetUID(), ResourceVersion: m.GetResourceVersion(),
		},
	}}
}

// isCappedStub reports whether obj is a stub for a refused object.
func isCappedStub(obj any) bool {
	_, ok := obj.(*cappedStub)
	return ok
}

// capTransform runs the watch mode's transform, then admits the object
// (see objectAdmission). The informer calls it when the object is queued,
// before the cache stores it, so a refused object is cached only as a
// stub. A later change of the object passes through again and is
// admitted once room has freed.
func capTransform(
	a *objectAdmission, base cache.TransformFunc,
) cache.TransformFunc {
	return func(obj any) (any, error) {
		obj, err := base(obj)
		if err != nil || a.admit(obj) {
			return obj, err
		}
		return newCappedStub(obj), nil
	}
}
