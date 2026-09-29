package kube

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Attribute names for storage entities.
const (
	AttrRequested   = "storage.requested"
	AttrCapacity    = "storage.capacity"
	AttrAccessModes = "access.modes"
	AttrProvisioner = "provisioner"
)

// PVCSchema describes PersistentVolumeClaims.
type PVCSchema struct{}

// Kind implements Schema.
func (PVCSchema) Kind() knowledge.Kind { return KindPVC }

// RelationTypes implements Schema.
func (PVCSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{knowledge.References}
}

// Describe implements Schema.
func (PVCSchema) Describe(obj any) (Description, bool) {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]knowledge.Value{
		AttrPhase:    knowledge.Text(string(pvc.Status.Phase)),
		AttrDeleting: knowledge.Bool(pvc.DeletionTimestamp != nil),
		AttrAccessModes: knowledge.Text(
			accessModes(pvc.Spec.AccessModes)),
	}
	setQuantity(attrs, AttrRequested,
		pvc.Spec.Resources.Requests.Storage())
	setQuantity(attrs, AttrCapacity, pvc.Status.Capacity.Storage())
	conditions := make([]condition, 0, len(pvc.Status.Conditions))
	for _, c := range pvc.Status.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
	rel := relations{}
	rel.add(knowledge.References,
		knowledge.NewEntityID(KindPV, "", pvc.Spec.VolumeName))
	if pvc.Spec.StorageClassName != nil {
		rel.add(knowledge.References, knowledge.NewEntityID(
			KindStorageClass, "", *pvc.Spec.StorageClassName))
	}
	return Description{
		ID: objectID(KindPVC, pvc), UID: string(pvc.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema: resize requests.
func (PVCSchema) Diff(old, new any) []knowledge.FieldChange {
	before, ok1 := old.(*corev1.PersistentVolumeClaim)
	after, ok2 := new.(*corev1.PersistentVolumeClaim)
	if !ok1 || !ok2 {
		return nil
	}
	b := quantityText(*before.Spec.Resources.Requests.Storage())
	a := quantityText(*after.Spec.Resources.Requests.Storage())
	if b == a {
		return nil
	}
	return []knowledge.FieldChange{{
		Path: "spec.resources.requests.storage", Before: b, After: a,
	}}
}

// PVSchema describes PersistentVolumes.
type PVSchema struct{}

// Kind implements Schema.
func (PVSchema) Kind() knowledge.Kind { return KindPV }

// RelationTypes implements Schema.
func (PVSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{knowledge.References}
}

// Describe implements Schema.
func (PVSchema) Describe(obj any) (Description, bool) {
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]knowledge.Value{
		AttrPhase: knowledge.Text(string(pv.Status.Phase)),
	}
	if pv.Status.Reason != "" {
		attrs[AttrReason] = knowledge.Text(pv.Status.Reason)
	}
	if pv.Status.Message != "" {
		attrs[AttrMessage] = knowledge.Text(truncate(pv.Status.Message))
	}
	setQuantity(attrs, AttrCapacity, pv.Spec.Capacity.Storage())
	rel := relations{}
	rel.add(knowledge.References, knowledge.NewEntityID(
		KindStorageClass, "", pv.Spec.StorageClassName))
	return Description{
		ID: objectID(KindPV, pv), UID: string(pv.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema.
func (PVSchema) Diff(_, _ any) []knowledge.FieldChange { return nil }

// StorageClassSchema describes StorageClasses.
type StorageClassSchema struct{}

// Kind implements Schema.
func (StorageClassSchema) Kind() knowledge.Kind { return KindStorageClass }

// RelationTypes implements Schema.
func (StorageClassSchema) RelationTypes() []knowledge.RelationType {
	return nil
}

// Describe implements Schema.
func (StorageClassSchema) Describe(obj any) (Description, bool) {
	sc, ok := obj.(*storagev1.StorageClass)
	if !ok {
		return Description{}, false
	}
	return Description{
		ID: objectID(KindStorageClass, sc), UID: string(sc.UID),
		Attributes: map[string]knowledge.Value{
			AttrProvisioner: knowledge.Text(sc.Provisioner),
		},
	}, true
}

// Diff implements Schema.
func (StorageClassSchema) Diff(_, _ any) []knowledge.FieldChange {
	return nil
}

// NamespaceSchema describes Namespaces.
type NamespaceSchema struct{}

// Kind implements Schema.
func (NamespaceSchema) Kind() knowledge.Kind { return KindNamespace }

// RelationTypes implements Schema.
func (NamespaceSchema) RelationTypes() []knowledge.RelationType {
	return nil
}

// Describe implements Schema.
func (NamespaceSchema) Describe(obj any) (Description, bool) {
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]knowledge.Value{
		AttrPhase:  knowledge.Text(string(ns.Status.Phase)),
		AttrLabels: knowledge.Text(labelText(ns.Labels)),
		AttrPodSecurityInvalid: knowledge.Text(
			podSecurityProblem(ns.Labels)),
	}
	conditions := make([]condition, 0, len(ns.Status.Conditions))
	for _, c := range ns.Status.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
	return Description{
		ID: objectID(KindNamespace, ns), UID: string(ns.UID),
		Attributes: attrs,
	}, true
}

// Diff implements Schema.
func (NamespaceSchema) Diff(_, _ any) []knowledge.FieldChange { return nil }

func accessModes(modes []corev1.PersistentVolumeAccessMode) string {
	out := ""
	for i, mode := range modes {
		if i > 0 {
			out += ","
		}
		out += string(mode)
	}
	return out
}
