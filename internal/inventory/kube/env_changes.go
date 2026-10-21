package kube

import (
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// envValue is what one environment variable holds: a literal, or a
// reference to a Secret, a ConfigMap or the pod itself. The origin is kept
// beside the text so a literal that reads like a reference is still a
// literal.
type envValue struct {
	text      string
	reference bool
}

// shown is the value for a message: a reference names its object and is
// shown as it is, a literal only when its name and shape are harmless.
func (v envValue) shown(name string) string {
	if v.reference {
		return v.text
	}
	return shownValue(name, v.text)
}

// envChanges reports each changed variable by name. A literal value is
// shown only when the name and the value both look harmless (see
// env_visibility.go); every other literal only says "changed" (an edit of
// two hidden values keeps both sides, see inventory.HiddenValue).
// References to a Secret or ConfigMap show the object and key, never a
// value.
func envChanges(
	path string, before, after []corev1.EnvVar,
) []inventory.FieldChange {
	old, next := envValues(before), envValues(after)
	names := map[string]bool{}
	for name := range old {
		names[name] = true
	}
	for name := range next {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	var fields []inventory.FieldChange
	for _, name := range sorted {
		b, a := old[name], next[name]
		if b == a {
			continue
		}
		fields = append(fields, inventory.FieldChange{
			Path: path + ".env." + name, Before: b.shown(name),
			After: a.shown(name)})
	}
	return fields
}

func envValues(vars []corev1.EnvVar) map[string]envValue {
	out := make(map[string]envValue, len(vars))
	for _, v := range vars {
		out[v.Name] = envValueOf(v)
	}
	return out
}

func envValueOf(v corev1.EnvVar) envValue {
	from := v.ValueFrom
	switch {
	case from == nil:
		return envValue{text: v.Value}
	case from.SecretKeyRef != nil:
		return envReference("secret " + from.SecretKeyRef.Name +
			"/" + from.SecretKeyRef.Key)
	case from.ConfigMapKeyRef != nil:
		return envReference("config map " + from.ConfigMapKeyRef.Name +
			"/" + from.ConfigMapKeyRef.Key)
	case from.FieldRef != nil:
		return envReference("field " + from.FieldRef.FieldPath)
	}
	return envReference("resource")
}

func envReference(what string) envValue {
	return envValue{text: "from " + what, reference: true}
}
