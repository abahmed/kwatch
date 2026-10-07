package kube

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// setDeletion marks an object that is being deleted, with when deletion
// was requested and which finalizers still hold it. An object that is
// not being deleted gets none of them, so
// the finalizers every healthy claim carries never enter the model.
func setDeletion(attrs map[string]inventory.Value, obj metav1.Object) {
	deleting := obj.GetDeletionTimestamp()
	if deleting == nil {
		return
	}
	attrs[AttrDeleting] = inventory.Bool(true)
	attrs[AttrDeletingSince] = inventory.Time(deleting.Time)
	setFinalizers(attrs, obj)
}

// setFinalizers records the finalizers that hold a deletion; a pod
// keeps its own deletion times and records only these.
func setFinalizers(attrs map[string]inventory.Value, obj metav1.Object) {
	if obj.GetDeletionTimestamp() == nil {
		return
	}
	if finalizers := obj.GetFinalizers(); len(finalizers) > 0 {
		attrs[AttrFinalizers] = inventory.Text(
			evidenceText(strings.Join(finalizers, ",")))
	}
}

// setNamespaceFinalizers adds the finalizers in spec, which hold a
// terminating namespace until its content is gone, to the ones in its
// metadata.
func setNamespaceFinalizers(
	attrs map[string]inventory.Value, ns *corev1.Namespace,
) {
	if ns.DeletionTimestamp == nil || len(ns.Spec.Finalizers) == 0 {
		return
	}
	names := append([]string{}, ns.Finalizers...)
	for _, name := range ns.Spec.Finalizers {
		names = append(names, string(name))
	}
	attrs[AttrFinalizers] = inventory.Text(
		evidenceText(strings.Join(names, ",")))
}
