package kube

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrFrozenConfig lists the ConfigMaps and Secrets whose content a
// pod reads once, when a container starts: "configmap/api-config,
// secret/api-token". Environment variables and subPath mounts never
// refresh, so a pod started before a change keeps the old content until
// its containers restart. A plain volume mount is left out: the kubelet
// updates it in place.
const AttrFrozenConfig = "config.frozen"

// maxFrozenRefs bounds the references kept for one pod.
const maxFrozenRefs = 8

// frozenConfig lists the pod's frozen references, sorted and without
// repeats, as "kind/name" joined by commas.
func frozenConfig(pod *corev1.Pod) string {
	seen := map[string]bool{}
	add := func(kind inventory.Kind, name string, _ *bool) {
		seen[string(kind)+"/"+name] = true
	}
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		containerReferences(c, add)
		for _, mount := range c.VolumeMounts {
			if mount.SubPath == "" && mount.SubPathExpr == "" {
				continue
			}
			for _, volume := range pod.Spec.Volumes {
				if volume.Name == mount.Name {
					volumeReferences(volume, add)
				}
			}
		}
	})
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	if len(refs) > maxFrozenRefs {
		refs = refs[:maxFrozenRefs]
	}
	return strings.Join(refs, ",")
}
