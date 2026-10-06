package kube

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// containerFieldChanges reports what changed inside the containers of two
// pod templates: environment, command, arguments, probes, ports and the
// config a container loads whole. Images and resources are reported by
// their own helpers. A container that appeared or disappeared is one
// field of its own.
func containerFieldChanges(
	before, after *corev1.PodTemplateSpec,
) []inventory.FieldChange {
	old, next := containersByName(before), containersByName(after)
	var fields []inventory.FieldChange
	for _, name := range containerNames(old, next) {
		prev, hadBefore := old[name]
		cur, hasAfter := next[name]
		path := "containers[" + name + "]"
		switch {
		case !hadBefore:
			fields = append(fields, inventory.FieldChange{
				Path: path, After: "added"})
		case !hasAfter:
			fields = append(fields, inventory.FieldChange{
				Path: path, Before: "removed"})
		default:
			fields = append(fields, containerDiff(path, prev, cur)...)
		}
	}
	return fields
}

func containersByName(
	template *corev1.PodTemplateSpec,
) map[string]corev1.Container {
	out := make(map[string]corev1.Container)
	forEachContainer(&corev1.Pod{Spec: template.Spec},
		func(c corev1.Container, _ bool) { out[c.Name] = c })
	return out
}

func containerNames(a, b map[string]corev1.Container) []string {
	seen := map[string]bool{}
	var names []string
	for _, set := range []map[string]corev1.Container{a, b} {
		for name := range set {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func containerDiff(
	path string, before, after corev1.Container,
) []inventory.FieldChange {
	fields := envChanges(path, before.Env, after.Env)
	fields = appendText(fields, path+".command",
		strings.Join(before.Command, " "), strings.Join(after.Command, " "))
	fields = appendText(fields, path+".args",
		strings.Join(before.Args, " "), strings.Join(after.Args, " "))
	fields = appendText(fields, path+".envFrom",
		envFromText(before.EnvFrom), envFromText(after.EnvFrom))
	fields = appendText(fields, path+".ports",
		portsText(before.Ports), portsText(after.Ports))
	for _, probe := range []struct {
		name string
		old  *corev1.Probe
		next *corev1.Probe
	}{
		{"livenessProbe", before.LivenessProbe, after.LivenessProbe},
		{"readinessProbe", before.ReadinessProbe, after.ReadinessProbe},
		{"startupProbe", before.StartupProbe, after.StartupProbe},
	} {
		fields = appendText(fields, path+"."+probe.name,
			probeText(probe.old), probeText(probe.next))
	}
	return fields
}

func appendText(
	fields []inventory.FieldChange, path, before, after string,
) []inventory.FieldChange {
	if before == after {
		return fields
	}
	return append(fields, inventory.FieldChange{
		Path: path, Before: before, After: after})
}

// envChanges reports each changed variable by name. A literal value is
// shown, except for names that look like credentials: those only say
// "changed". References to a Secret or ConfigMap show the object and key,
// never a value.
func envChanges(
	path string, before, after []corev1.EnvVar,
) []inventory.FieldChange {
	old, next := envTexts(before), envTexts(after)
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
		if sensitiveName(name) && !isReference(b) && !isReference(a) {
			b, a = hidden(b), hidden(a)
		}
		fields = append(fields, inventory.FieldChange{
			Path: path + ".env." + name, Before: b, After: a})
	}
	return fields
}

// hidden keeps whether a value existed, never what it was.
func hidden(value string) string {
	if value == "" {
		return ""
	}
	return "changed"
}

const referencePrefix = "from "

func isReference(value string) bool {
	return strings.HasPrefix(value, referencePrefix)
}

func envTexts(vars []corev1.EnvVar) map[string]string {
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		out[v.Name] = envText(v)
	}
	return out
}

func envText(v corev1.EnvVar) string {
	from := v.ValueFrom
	switch {
	case from == nil:
		return v.Value
	case from.SecretKeyRef != nil:
		return referencePrefix + "secret " + from.SecretKeyRef.Name +
			"/" + from.SecretKeyRef.Key
	case from.ConfigMapKeyRef != nil:
		return referencePrefix + "config map " + from.ConfigMapKeyRef.Name +
			"/" + from.ConfigMapKeyRef.Key
	case from.FieldRef != nil:
		return referencePrefix + "field " + from.FieldRef.FieldPath
	}
	return referencePrefix + "resource"
}

// sensitiveWords mark an environment variable whose literal value is
// probably a credential.
var sensitiveWords = []string{
	"password", "passwd", "secret", "token", "key", "credential", "auth",
	"cert", "dsn",
}

func sensitiveName(name string) bool {
	lower := strings.ToLower(name)
	for _, word := range sensitiveWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func envFromText(sources []corev1.EnvFromSource) string {
	var parts []string
	for _, source := range sources {
		if ref := source.ConfigMapRef; ref != nil {
			parts = append(parts, "configmap/"+ref.Name)
		}
		if ref := source.SecretRef; ref != nil {
			parts = append(parts, "secret/"+ref.Name)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func portsText(ports []corev1.ContainerPort) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		protocol := string(p.Protocol)
		if protocol == "" {
			protocol = "TCP"
		}
		parts = append(parts, strconv.Itoa(int(p.ContainerPort))+"/"+protocol)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// probeText is a probe in one short phrase: what it checks and how
// often. A missing probe is empty.
func probeText(p *corev1.Probe) string {
	if p == nil {
		return ""
	}
	var what string
	switch {
	case p.HTTPGet != nil:
		what = "http " + p.HTTPGet.Path + " :" + p.HTTPGet.Port.String()
	case p.TCPSocket != nil:
		what = "tcp :" + p.TCPSocket.Port.String()
	case p.Exec != nil:
		what = "exec " + strings.Join(p.Exec.Command, " ")
	case p.GRPC != nil:
		what = "grpc :" + strconv.Itoa(int(p.GRPC.Port))
	}
	if what != "" {
		what += " "
	}
	return what + "every " + strconv.Itoa(int(p.PeriodSeconds)) +
		"s, timeout " + strconv.Itoa(int(p.TimeoutSeconds)) + "s, " +
		strconv.Itoa(int(p.FailureThreshold)) + " failures"
}

// volumeChanges reports volumes by name: what each one is backed by.
func volumeChanges(
	before, after *corev1.PodTemplateSpec,
) []inventory.FieldChange {
	old, next := volumeTexts(before), volumeTexts(after)
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
		fields = appendText(fields, "volumes["+name+"]",
			old[name], next[name])
	}
	return fields
}

func volumeTexts(template *corev1.PodTemplateSpec) map[string]string {
	out := make(map[string]string, len(template.Spec.Volumes))
	for _, v := range template.Spec.Volumes {
		out[v.Name] = volumeSource(v)
	}
	return out
}

func volumeSource(v corev1.Volume) string {
	switch {
	case v.ConfigMap != nil:
		return "configmap/" + v.ConfigMap.Name
	case v.Secret != nil:
		return "secret/" + v.Secret.SecretName
	case v.PersistentVolumeClaim != nil:
		return "pvc/" + v.PersistentVolumeClaim.ClaimName
	case v.EmptyDir != nil:
		return "emptyDir"
	case v.HostPath != nil:
		return "hostPath " + v.HostPath.Path
	}
	return "other"
}
