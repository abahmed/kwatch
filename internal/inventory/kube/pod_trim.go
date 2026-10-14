package kube

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// trimPod removes from a cached Pod the fields no reader looks at, so a
// 20,000-pod cluster fits the informer cache in far less memory. Every
// rule below mirrors one reader; pod_trim_test.go proves a trimmed pod
// gives the same observations as the original.
//
//   - environment: a value stays only when a reader can use it (a
//     driver-named variable, an external endpoint, a Service address);
//     a reference stays only when it names a Secret or ConfigMap
//   - volumes: only Secret, ConfigMap, claim and projected-source
//     references stay
//   - mounts: only the sub-path mounts the frozen-config check reads
//   - arguments of regular containers: only driver or provisioner flags
//   - lifecycle hooks and other container defaults nothing reads
func trimPod(pod *corev1.Pod) {
	for i := range pod.Spec.InitContainers {
		trimContainer(&pod.Spec.InitContainers[i], pod.Namespace, true)
	}
	for i := range pod.Spec.Containers {
		trimContainer(&pod.Spec.Containers[i], pod.Namespace, false)
	}
	for i := range pod.Spec.Volumes {
		pod.Spec.Volumes[i] = trimVolume(pod.Spec.Volumes[i])
	}
}

func trimContainer(c *corev1.Container, namespace string, initial bool) {
	c.Env = trimEnv(c.Env, namespace)
	c.Lifecycle = nil
	c.TerminationMessagePath = ""
	c.TerminationMessagePolicy = ""
	c.ImagePullPolicy = ""
	c.WorkingDir = ""
	c.VolumeDevices = nil
	c.ResizePolicy = nil
	c.VolumeMounts = trimMounts(c.VolumeMounts)
	if !initial {
		// Init containers keep their words: init_wait.go reads them all.
		c.Command = driverWords(c.Command, c.Args)
		c.Args = nil
	}
}

// trimMounts keeps the sub-path mounts, the only ones read.
func trimMounts(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	var kept []corev1.VolumeMount
	for _, m := range mounts {
		if m.SubPath != "" || m.SubPathExpr != "" {
			kept = append(kept, corev1.VolumeMount{
				Name: m.Name, SubPath: m.SubPath,
				SubPathExpr: m.SubPathExpr,
			})
		}
	}
	return kept
}

// trimEnv drops the variables nothing reads and the values of the rest.
func trimEnv(env []corev1.EnvVar, namespace string) []corev1.EnvVar {
	var kept []corev1.EnvVar
	for _, e := range env {
		if e.ValueFrom != nil {
			if from := trimEnvSource(e.ValueFrom); from != nil {
				kept = append(kept, corev1.EnvVar{
					Name: e.Name, ValueFrom: from,
				})
			}
			continue
		}
		if envValueRead(e.Name, e.Value, namespace) {
			kept = append(kept, corev1.EnvVar{Name: e.Name, Value: e.Value})
		}
	}
	return kept
}

// envValueRead reports whether a reader uses the value: the driver
// names, the external endpoints and the Service calls.
func envValueRead(name, value, namespace string) bool {
	if namesDriver(name) {
		return true
	}
	if _, ok := dependencyIn(value); ok {
		return true
	}
	_, ok := serviceCallIn(name, value, namespace)
	return ok
}

// trimEnvSource keeps the Secret and ConfigMap references of a source.
func trimEnvSource(from *corev1.EnvVarSource) *corev1.EnvVarSource {
	out := &corev1.EnvVarSource{}
	if ref := from.SecretKeyRef; ref != nil {
		out.SecretKeyRef = &corev1.SecretKeySelector{
			LocalObjectReference: ref.LocalObjectReference,
			Optional:             ref.Optional,
		}
	}
	if ref := from.ConfigMapKeyRef; ref != nil {
		out.ConfigMapKeyRef = &corev1.ConfigMapKeySelector{
			LocalObjectReference: ref.LocalObjectReference,
			Optional:             ref.Optional,
		}
	}
	if out.SecretKeyRef == nil && out.ConfigMapKeyRef == nil {
		return nil
	}
	return out
}

// driverWords keeps, from the command and arguments read as one list,
// the driver or provisioner flags and the value that follows one.
func driverWords(command, args []string) []string {
	words := append(append([]string{}, command...), args...)
	var kept []string
	for i, word := range words {
		flag, _, hasValue := strings.Cut(word, "=")
		if !strings.HasPrefix(flag, "-") || !namesDriver(flag) {
			continue
		}
		kept = append(kept, word)
		if !hasValue && i+1 < len(words) && !strings.HasPrefix(words[i+1], "-") {
			kept = append(kept, words[i+1])
		}
	}
	return kept
}

// trimVolume keeps the sources that name a Secret, ConfigMap or claim.
func trimVolume(v corev1.Volume) corev1.Volume {
	out := corev1.Volume{Name: v.Name}
	switch {
	case v.Secret != nil:
		out.Secret = &corev1.SecretVolumeSource{
			SecretName: v.Secret.SecretName, Optional: v.Secret.Optional,
		}
	case v.ConfigMap != nil:
		out.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: v.ConfigMap.LocalObjectReference,
			Optional:             v.ConfigMap.Optional,
		}
	case v.PersistentVolumeClaim != nil:
		out.PersistentVolumeClaim = v.PersistentVolumeClaim
	case v.Projected != nil:
		out.Projected = trimProjected(v.Projected)
	}
	return out
}

// trimProjected keeps the Secret and ConfigMap sources of a projected
// volume.
func trimProjected(
	p *corev1.ProjectedVolumeSource,
) *corev1.ProjectedVolumeSource {
	out := &corev1.ProjectedVolumeSource{}
	for _, s := range p.Sources {
		var kept corev1.VolumeProjection
		if s.Secret != nil {
			kept.Secret = &corev1.SecretProjection{
				LocalObjectReference: s.Secret.LocalObjectReference,
				Optional:             s.Secret.Optional,
			}
		}
		if s.ConfigMap != nil {
			kept.ConfigMap = &corev1.ConfigMapProjection{
				LocalObjectReference: s.ConfigMap.LocalObjectReference,
				Optional:             s.ConfigMap.Optional,
			}
		}
		if kept.Secret != nil || kept.ConfigMap != nil {
			out.Sources = append(out.Sources, kept)
		}
	}
	return out
}
