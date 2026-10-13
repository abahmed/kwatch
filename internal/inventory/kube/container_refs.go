package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// RefAttr names the attribute that says where a container (or a pod)
// refers to a Secret or ConfigMap: "ref.secret/stripe-api". Its value
// is the field path of the reference, such as
// "env[STRIPE_KEY].valueFrom.secretKeyRef" or "envFrom.secretRef". The
// path names a key, never a value.
func RefAttr(kind inventory.Kind, name string) string {
	return "ref." + string(kind) + "/" + name
}

// setRefAttributes records where the container refers to each Secret
// and ConfigMap: the first reference to each object wins.
func setRefAttributes(attrs map[string]inventory.Value, c corev1.Container) {
	add := func(kind inventory.Kind, name, path string) {
		key := RefAttr(kind, name)
		if _, seen := attrs[key]; !seen && name != "" {
			attrs[key] = inventory.Text(path)
		}
	}
	for _, from := range c.EnvFrom {
		if ref := from.SecretRef; ref != nil {
			add(KindSecret, ref.Name, "envFrom.secretRef")
		}
		if ref := from.ConfigMapRef; ref != nil {
			add(KindConfigMap, ref.Name, "envFrom.configMapRef")
		}
	}
	for _, env := range c.Env {
		if env.ValueFrom == nil {
			continue
		}
		base := "env[" + env.Name + "].valueFrom."
		if ref := env.ValueFrom.SecretKeyRef; ref != nil {
			add(KindSecret, ref.Name, base+"secretKeyRef")
		}
		if ref := env.ValueFrom.ConfigMapKeyRef; ref != nil {
			add(KindConfigMap, ref.Name, base+"configMapKeyRef")
		}
	}
}

// setVolumeRefAttributes records, on the pod, where its volumes refer
// to each Secret and ConfigMap, in the form of RefAttr:
// "volumes[certs].secret".
func setVolumeRefAttributes(
	attrs map[string]inventory.Value, pod *corev1.Pod,
) {
	add := func(kind inventory.Kind, name, path string) {
		key := RefAttr(kind, name)
		if _, seen := attrs[key]; !seen && name != "" {
			attrs[key] = inventory.Text(path)
		}
	}
	for _, volume := range pod.Spec.Volumes {
		base := "volumes[" + volume.Name + "]."
		switch {
		case volume.Secret != nil:
			add(KindSecret, volume.Secret.SecretName, base+"secret")
		case volume.ConfigMap != nil:
			add(KindConfigMap, volume.ConfigMap.Name, base+"configMap")
		case volume.Projected != nil:
			addProjectedRefs(volume.Projected, base, add)
		}
	}
}

// addProjectedRefs adds the Secrets and ConfigMaps a projected volume
// combines.
func addProjectedRefs(
	projected *corev1.ProjectedVolumeSource, base string,
	add func(inventory.Kind, string, string),
) {
	for _, source := range projected.Sources {
		if source.Secret != nil {
			add(KindSecret, source.Secret.Name,
				base+"projected.sources[].secret")
		}
		if source.ConfigMap != nil {
			add(KindConfigMap, source.ConfigMap.Name,
				base+"projected.sources[].configMap")
		}
	}
}
