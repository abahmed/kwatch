package handler

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
)

// DetectPodReferenceIssues turns opaque admission/mount failures into precise
// signals when the referenced object is provably absent from the informer
// cache. Optional references and an unavailable lister are intentionally
// ignored: absence is only actionable when Kubernetes requires the reference.
func DetectPodReferenceIssues(pod *corev1.Pod, listers Listers) []*event.Signal {
	if pod == nil {
		return nil
	}
	var signals []*event.Signal
	if listers.Secret != nil {
		for _, ref := range requiredSecretReferences(pod) {
			if _, err := listers.Secret.Secrets(pod.Namespace).Get(ref); errors.IsNotFound(err) {
				signals = append(signals, podReferenceSignal(pod, constant.ReasonProjectedSecretMissing, "Secret", ref))
			}
		}
	}
	if listers.ConfigMap != nil {
		for _, ref := range requiredConfigMapReferences(pod) {
			if _, err := listers.ConfigMap.ConfigMaps(pod.Namespace).Get(ref); errors.IsNotFound(err) {
				signals = append(signals, podReferenceSignal(pod, constant.ReasonProjectedConfigMapMissing, "ConfigMap", ref))
			}
		}
	}
	if listers.ServiceAccount != nil {
		serviceAccount := pod.Spec.ServiceAccountName
		if serviceAccount == "" {
			serviceAccount = "default"
		}
		if _, err := listers.ServiceAccount.ServiceAccounts(pod.Namespace).Get(serviceAccount); errors.IsNotFound(err) {
			signals = append(signals, podReferenceSignal(pod, constant.ReasonServiceAccountMissing, "ServiceAccount", serviceAccount))
		}
	}
	return signals
}

func requiredSecretReferences(pod *corev1.Pod) []string {
	refs := make([]string, 0)
	for _, ref := range pod.Spec.ImagePullSecrets {
		if ref.Name != "" {
			refs = append(refs, ref.Name)
		}
	}
	for _, volume := range pod.Spec.Volumes {
		refs = append(refs, requiredSecretVolumeReferences(volume)...)
	}
	for _, container := range append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...) {
		refs = append(refs, requiredSecretEnvReferences(container)...)
	}
	return uniqueNames(refs)
}

func requiredSecretVolumeReferences(volume corev1.Volume) []string {
	var refs []string
	if volume.Secret != nil && requiredReference(volume.Secret.Optional) {
		refs = append(refs, volume.Secret.SecretName)
	}
	if volume.Projected != nil {
		for _, source := range volume.Projected.Sources {
			if source.Secret != nil && requiredReference(source.Secret.Optional) {
				refs = append(refs, source.Secret.Name)
			}
		}
	}
	return refs
}

func requiredSecretEnvReferences(container corev1.Container) []string {
	var refs []string
	for _, env := range container.EnvFrom {
		if env.SecretRef != nil && requiredReference(env.SecretRef.Optional) {
			refs = append(refs, env.SecretRef.Name)
		}
	}
	for _, env := range container.Env {
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && requiredReference(env.ValueFrom.SecretKeyRef.Optional) {
			refs = append(refs, env.ValueFrom.SecretKeyRef.Name)
		}
	}
	return refs
}

func requiredReference(optional *bool) bool { return optional == nil || !*optional }

func requiredConfigMapReferences(pod *corev1.Pod) []string {
	refs := make([]string, 0)
	for _, volume := range pod.Spec.Volumes {
		refs = append(refs, requiredConfigMapVolumeReferences(volume)...)
	}
	for _, container := range append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...) {
		refs = append(refs, requiredConfigMapEnvReferences(container)...)
	}
	return uniqueNames(refs)
}

func requiredConfigMapVolumeReferences(volume corev1.Volume) []string {
	var refs []string
	if volume.ConfigMap != nil && requiredReference(volume.ConfigMap.Optional) {
		refs = append(refs, volume.ConfigMap.Name)
	}
	if volume.Projected != nil {
		for _, source := range volume.Projected.Sources {
			if source.ConfigMap != nil && requiredReference(source.ConfigMap.Optional) {
				refs = append(refs, source.ConfigMap.Name)
			}
		}
	}
	return refs
}

func requiredConfigMapEnvReferences(container corev1.Container) []string {
	var refs []string
	for _, env := range container.EnvFrom {
		if env.ConfigMapRef != nil && requiredReference(env.ConfigMapRef.Optional) {
			refs = append(refs, env.ConfigMapRef.Name)
		}
	}
	for _, env := range container.Env {
		if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil && requiredReference(env.ValueFrom.ConfigMapKeyRef.Optional) {
			refs = append(refs, env.ValueFrom.ConfigMapKeyRef.Name)
		}
	}
	return refs
}

func uniqueNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				result = append(result, name)
			}
		}
	}
	return result
}

func podReferenceSignal(pod *corev1.Pod, reason, kind, name string) *event.Signal {
	return &event.Signal{Resource: "pod", Namespace: pod.Namespace, PodName: pod.Name, NodeName: pod.Spec.NodeName, Owner: pod.Namespace + "/" + pod.Name, Reason: reason, Labels: pod.Labels, Hint: fmt.Sprintf("pod references required %s %q, but it is not present in the namespace", kind, name)}
}
