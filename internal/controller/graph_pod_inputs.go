package controller

import (
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

// podGraphInputsChanged reports whether an update touched anything the pod's
// graph edges are built from. A crash-looping pod produces a stream of status
// updates that change none of it.
func podGraphInputsChanged(old, new *corev1.Pod) bool {
	if old == nil || new == nil {
		return true
	}
	if old.Spec.NodeName != new.Spec.NodeName ||
		old.Spec.ServiceAccountName != new.Spec.ServiceAccountName ||
		!maps.Equal(old.Labels, new.Labels) ||
		!equality.Semantic.DeepEqual(
			old.OwnerReferences,
			new.OwnerReferences,
		) ||
		!equality.Semantic.DeepEqual(old.Spec.Volumes, new.Spec.Volumes) ||
		!equality.Semantic.DeepEqual(
			old.Spec.ImagePullSecrets, new.Spec.ImagePullSecrets,
		) {
		return true
	}
	return containerRefsChanged(old, new)
}

// containerRefsChanged reports whether any container's environment
// references moved. Those are the ConfigMap and Secret edges.
func containerRefsChanged(old, new *corev1.Pod) bool {
	if len(old.Spec.Containers) != len(new.Spec.Containers) ||
		len(old.Spec.InitContainers) != len(new.Spec.InitContainers) {
		return true
	}
	for i := range old.Spec.Containers {
		if containerEnvChanged(
			&old.Spec.Containers[i], &new.Spec.Containers[i],
		) {
			return true
		}
	}
	for i := range old.Spec.InitContainers {
		if containerEnvChanged(
			&old.Spec.InitContainers[i], &new.Spec.InitContainers[i],
		) {
			return true
		}
	}
	return false
}

func containerEnvChanged(old, new *corev1.Container) bool {
	return !equality.Semantic.DeepEqual(old.Env, new.Env) ||
		!equality.Semantic.DeepEqual(old.EnvFrom, new.EnvFrom)
}
