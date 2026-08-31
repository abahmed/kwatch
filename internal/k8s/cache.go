package k8s

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TrimManagedFields removes server-side apply ownership metadata before an
// object enters an informer cache. The transform runs before publication, so
// mutating the object in place is safe and avoids an extra deep copy.
func TrimManagedFields(obj interface{}) (interface{}, error) {
	if meta, ok := obj.(metav1.Object); ok {
		meta.SetManagedFields(nil)
	}
	return obj, nil
}
