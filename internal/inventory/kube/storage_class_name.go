package kube

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

// Annotations that mark the default StorageClass and, on older claims,
// name their class.
const (
	defaultClassAnnotation     = "storageclass.kubernetes.io/is-default-class"
	defaultClassBetaAnnotation = "storageclass.beta.kubernetes.io/" +
		"is-default-class"
	claimClassBetaAnnotation = "volume.beta.kubernetes.io/storage-class"
)

// claimClass is the class a claim names: spec.storageClassName, or the
// older annotation. named is false for a claim that sets neither.
func claimClass(pvc *corev1.PersistentVolumeClaim) (name string, named bool) {
	if pvc.Spec.StorageClassName != nil {
		return *pvc.Spec.StorageClassName, true
	}
	name, named = pvc.Annotations[claimClassBetaAnnotation]
	return name, named
}

// defaultClass reports a class marked as the cluster default.
func defaultClass(sc *storagev1.StorageClass) bool {
	return sc.Annotations[defaultClassAnnotation] == "true" ||
		sc.Annotations[defaultClassBetaAnnotation] == "true"
}
