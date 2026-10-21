package kube

import (
	"maps"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrManagedBy is the field manager that last wrote an object's spec.
// Links resolves it to a controller workload of the same name.
const AttrManagedBy = "managed.by"

// SpecManager returns the field manager of the most recent Apply or
// Update that was not a status write. Status writers (a kubelet, a
// controller reporting progress) describe an object; they do not manage
// it. Entries without timestamps count in list order.
func SpecManager(obj metav1.Object) string {
	var (
		manager string
		latest  *metav1.Time
	)
	for _, entry := range obj.GetManagedFields() {
		if !specWrite(entry) {
			continue
		}
		if manager == "" || entry.Time == nil || latest == nil ||
			!entry.Time.Before(latest) {
			manager, latest = entry.Manager, entry.Time
		}
	}
	return manager
}

func specWrite(entry metav1.ManagedFieldsEntry) bool {
	if entry.Manager == "" || entry.Subresource == "status" {
		return false
	}
	return entry.Operation == metav1.ManagedFieldsOperationApply ||
		entry.Operation == metav1.ManagedFieldsOperationUpdate
}

// link adds the generic link attributes every object carries, typed or
// unstructured, to desc: its spec manager and, while it is being
// deleted, the owners of the finalizers that hold it.
func link(obj any, desc *Description) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	attrs := map[string]inventory.Value{}
	if manager := SpecManager(accessor); manager != "" {
		attrs[AttrManagedBy] = inventory.Text(manager)
	}
	setFinalizerManagers(attrs, accessor)
	if len(attrs) == 0 {
		return
	}
	if desc.Attributes == nil {
		desc.Attributes = make(map[string]inventory.Value)
	}
	maps.Copy(desc.Attributes, attrs)
}
