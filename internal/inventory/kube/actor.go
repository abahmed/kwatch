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
// recent managed-fields entry, and the most recent spec write when that
// is a different entry, without their field sets. It preserves the actor
// for change attribution and the spec manager for managed-by links at a
// fraction of the memory.
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
	kept := []metav1.ManagedFieldsEntry{entries[latest]}
	if spec := latestSpecWrite(entries); spec >= 0 && spec != latest {
		// The spec manager feeds managed-by links. The latest entry stays
		// first, so Actor keeps choosing it when the times tie.
		kept = append(kept, entries[spec])
	}
	for i := range kept {
		kept[i].FieldsV1 = nil
	}
	meta.SetManagedFields(kept)
	return obj, nil
}

// latestSpecWrite returns the index of the most recent timed spec write,
// or -1.
func latestSpecWrite(entries []metav1.ManagedFieldsEntry) int {
	found := -1
	for i, entry := range entries {
		if entry.Time == nil || !specWrite(entry) {
			continue
		}
		if found < 0 || !entry.Time.Before(entries[found].Time) {
			found = i
		}
	}
	return found
}
