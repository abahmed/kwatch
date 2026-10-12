package kube

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// podSecurityPrefix starts the namespace labels that set the Pod
// Security Admission level, version and mode of a namespace.
const podSecurityPrefix = "pod-security.kubernetes.io/"

// serviceAccountChanges reports the pull secrets an account hands to its
// pods: a removed or renamed one is a common cause of image pull errors.
func serviceAccountChanges(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.ServiceAccount)
	after, ok2 := new.(*corev1.ServiceAccount)
	if !ok1 || !ok2 {
		return nil
	}
	return appendText(nil, "imagePullSecrets",
		pullSecretNames(before), pullSecretNames(after))
}

func pullSecretNames(account *corev1.ServiceAccount) string {
	names := make([]string, 0, len(account.ImagePullSecrets))
	for _, ref := range account.ImagePullSecrets {
		names = append(names, ref.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// storageClassChanges reports what decides how volumes are provisioned.
func storageClassChanges(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*storagev1.StorageClass)
	after, ok2 := new.(*storagev1.StorageClass)
	if !ok1 || !ok2 {
		return nil
	}
	var fields []inventory.FieldChange
	for _, c := range []struct{ path, before, after string }{
		{"provisioner", before.Provisioner, after.Provisioner},
		{"reclaimPolicy", reclaimPolicy(before), reclaimPolicy(after)},
		{"volumeBindingMode", bindingMode(before), bindingMode(after)},
		{"allowVolumeExpansion", expansionText(before),
			expansionText(after)},
		{"parameters", labelText(before.Parameters),
			labelText(after.Parameters)},
	} {
		fields = appendText(fields, c.path, c.before, c.after)
	}
	return fields
}

// reclaimPolicy is the class's policy; Kubernetes treats unset as Delete.
func reclaimPolicy(sc *storagev1.StorageClass) string {
	if sc.ReclaimPolicy == nil {
		return string(corev1.PersistentVolumeReclaimDelete)
	}
	return string(*sc.ReclaimPolicy)
}

func expansionText(sc *storagev1.StorageClass) string {
	allowed := sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion
	return strconv.FormatBool(allowed)
}

// namespaceChanges reports the Pod Security labels, which decide which
// pods the namespace admits.
func namespaceChanges(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.Namespace)
	after, ok2 := new.(*corev1.Namespace)
	if !ok1 || !ok2 {
		return nil
	}
	return appendText(nil, "labels.pod-security",
		podSecurityLabels(before), podSecurityLabels(after))
}

func podSecurityLabels(ns *corev1.Namespace) string {
	selected := map[string]string{}
	for key, value := range ns.Labels {
		if strings.HasPrefix(key, podSecurityPrefix) {
			selected[strings.TrimPrefix(key, podSecurityPrefix)] = value
		}
	}
	return labelText(selected)
}

// limitRangeChanges reports the limits that default, bound or reject
// the resources of every pod in the namespace.
func limitRangeChanges(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.LimitRange)
	after, ok2 := new.(*corev1.LimitRange)
	if !ok1 || !ok2 {
		return nil
	}
	return appendText(nil, "spec.limits",
		limitRangeText(before), limitRangeText(after))
}

func limitRangeText(lr *corev1.LimitRange) string {
	parts := make([]string, 0, len(lr.Spec.Limits))
	for _, item := range lr.Spec.Limits {
		parts = append(parts, string(item.Type)+" "+strings.Join([]string{
			"min " + resourceListText(item.Min),
			"max " + resourceListText(item.Max),
			"default " + resourceListText(item.Default),
			"defaultRequest " + resourceListText(item.DefaultRequest),
			"ratio " + resourceListText(item.MaxLimitRequestRatio),
		}, "; "))
	}
	sort.Strings(parts)
	return strings.Join(parts, " | ")
}
