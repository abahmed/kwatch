package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Actor returns the field manager of the most recent write, such as
// "kubectl-client-side-apply", "argocd-controller" or "kube-controller-
// manager". It is empty when managed fields were not retained.
func Actor(meta metav1.Object) string {
	var (
		manager string
		latest  metav1.Time
	)
	entries := meta.GetManagedFields()
	for _, entry := range entries {
		if entry.Time == nil {
			continue
		}
		if manager == "" || entry.Time.After(latest.Time) {
			manager, latest = entry.Manager, *entry.Time
		}
	}
	if manager == "" && len(entries) > 0 {
		// Without timestamps, the last entry is the most recent writer.
		manager = entries[len(entries)-1].Manager
	}
	return manager
}

// TrimToLatestManager is an informer transform that keeps only the most
// recent managed-fields entry, without its field set. It preserves the
// actor for change attribution at a fraction of the memory.
func TrimToLatestManager(obj any) (any, error) {
	meta, ok := obj.(metav1.Object)
	if !ok {
		return obj, nil
	}
	entries := meta.GetManagedFields()
	if len(entries) == 0 {
		return obj, nil
	}
	latest := -1
	for i, entry := range entries {
		if entry.Time == nil {
			continue
		}
		if latest < 0 || entry.Time.After(entries[latest].Time.Time) {
			latest = i
		}
	}
	if latest < 0 {
		meta.SetManagedFields(nil)
		return obj, nil
	}
	kept := entries[latest]
	kept.FieldsV1 = nil
	meta.SetManagedFields([]metav1.ManagedFieldsEntry{kept})
	return obj, nil
}
