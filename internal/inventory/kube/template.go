package kube

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// TemplateHash fingerprints a pod template. A rollout is a template change;
// replica and metadata edits leave the hash unchanged.
func TemplateHash(template *corev1.PodTemplateSpec) string {
	if template == nil {
		return ""
	}
	data, err := json.Marshal(template)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:6])
}

// templateDiff explains a template change in terms a reader recognises:
// images, referenced config, resources, probes. Anything else is reported
// as a generic template change.
func templateDiff(
	before, after *corev1.PodTemplateSpec,
) []inventory.FieldChange {
	if before == nil || after == nil ||
		TemplateHash(before) == TemplateHash(after) {
		return nil
	}
	var fields []inventory.FieldChange
	fields = append(fields, imageChanges(before, after)...)
	fields = append(fields, referenceChanges(before, after)...)
	oldPod := &corev1.Pod{Spec: before.Spec}
	newPod := &corev1.Pod{Spec: after.Spec}
	fields = append(fields, resourceDiff(oldPod, newPod)...)
	if restart := after.Annotations[restartedAtAnnotation]; restart !=
		before.Annotations[restartedAtAnnotation] {
		fields = append(fields, inventory.FieldChange{
			Path: "template.restartedAt", After: restart,
		})
	}
	if len(fields) == 0 {
		fields = append(fields, inventory.FieldChange{
			Path:   "spec.template",
			Before: TemplateHash(before), After: TemplateHash(after),
		})
	}
	return fields
}

const restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"

func imageChanges(
	before, after *corev1.PodTemplateSpec,
) []inventory.FieldChange {
	old := containerImages(before)
	next := containerImages(after)
	var fields []inventory.FieldChange
	for _, name := range sortedKeys(next) {
		image := next[name]
		if previous := old[name]; previous != image {
			fields = append(fields, inventory.FieldChange{
				Path:   "containers[" + name + "].image",
				Before: previous, After: image,
			})
		}
	}
	return fields
}

func containerImages(
	template *corev1.PodTemplateSpec,
) map[string]string {
	images := make(map[string]string)
	pod := &corev1.Pod{Spec: template.Spec}
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		images[c.Name] = c.Image
	})
	return images
}

func referenceChanges(
	before, after *corev1.PodTemplateSpec,
) []inventory.FieldChange {
	old := referenceSet(before)
	next := referenceSet(after)
	if old == next {
		return nil
	}
	return []inventory.FieldChange{{
		Path: "template.references", Before: old, After: next,
	}}
}

func referenceSet(template *corev1.PodTemplateSpec) string {
	ids := podReferences(&corev1.Pod{Spec: template.Spec})
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, string(id.Kind)+"/"+id.Name)
	}
	sort.Strings(names)
	return strings.Join(uniqueStrings(names), ",")
}

func uniqueStrings(values []string) []string {
	out := values[:0]
	for i, value := range values {
		if i == 0 || values[i-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
