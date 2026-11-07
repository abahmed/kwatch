package kube

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// configTargetsAnnotation holds, on the cached copy of a ConfigMap, the
// dependency targets read from its values before they were digested:
// "KEY=host:port" entries joined by commas. It is set by the informer
// transform only; a live object never carries it.
const configTargetsAnnotation = "kwatch.dev/dependency-targets"

// AttrConfigTargets is a ConfigMap's dependency targets, as sorted
// "KEY=host:port" entries joined by commas. Only the host and port of a
// value that names an endpoint outside the cluster is kept, never the
// value itself, and never from a key named like a credential.
const AttrConfigTargets = "config.targets"

// AttrConfigEnvRefs is the ConfigMaps a pod reads through its
// environment (envFrom and configMapKeyRef): "name" for all keys,
// "name/KEY" for one, sorted and joined by commas.
const AttrConfigEnvRefs = "config.env"

// AttrEndpointSources says where the pods that call an external endpoint
// read its address from: "namespace/name#KEY" entries, sorted, at most
// maxEndpointSources.
const AttrEndpointSources = "endpoint.sources"

// maxConfigValue bounds the value kwatch looks at for a dependency: a
// host:port or a URL is short, a config file or certificate is not.
const maxConfigValue = 512

// maxEndpointSources bounds the keys one endpoint remembers.
const maxEndpointSources = 5

// configTarget is one key of a ConfigMap whose value names an endpoint.
type configTarget struct {
	key      string
	endpoint string
}

func (t configTarget) String() string { return t.key + "=" + t.endpoint }

// configMapTargets reads the endpoints a ConfigMap's values name, with
// the parsing, bounds and credential filtering of a pod's environment
// values (dependencyIn, secretNamed). The first maxPodDependencies by
// key are kept.
func configMapTargets(cm *corev1.ConfigMap) []configTarget {
	keys := make([]string, 0, len(cm.Data))
	for key := range cm.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []configTarget
	for _, key := range keys {
		value := cm.Data[key]
		if secretNamed(key) || len(value) > maxConfigValue {
			continue
		}
		if endpoint, ok := dependencyIn(value); ok {
			out = append(out, configTarget{key: key, endpoint: endpoint})
		}
		if len(out) >= maxPodDependencies {
			break
		}
	}
	return out
}

// targetsText is the annotation and attribute value for targets.
func targetsText(targets []configTarget) string {
	parts := make([]string, len(targets))
	for i, t := range targets {
		parts[i] = t.String()
	}
	return strings.Join(parts, ",")
}

// parseTargets reads targetsText back; an entry that is not a key and a
// host:port is skipped, whatever wrote it.
func parseTargets(text string) []configTarget {
	var out []configTarget
	for _, entry := range strings.Split(text, ",") {
		key, endpoint, ok := strings.Cut(entry, "=")
		if !ok || key == "" || !hostPort.MatchString(endpoint) {
			continue
		}
		out = append(out, configTarget{key: key, endpoint: endpoint})
	}
	return out
}

// envConfigRefs lists the ConfigMaps a pod's containers read through
// their environment.
func envConfigRefs(pod *corev1.Pod) []string {
	seen := map[string]bool{}
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		for _, from := range c.EnvFrom {
			if ref := from.ConfigMapRef; ref != nil {
				seen[ref.Name] = true
			}
		}
		for _, env := range c.Env {
			if env.ValueFrom == nil || env.ValueFrom.ConfigMapKeyRef == nil {
				continue
			}
			ref := env.ValueFrom.ConfigMapKeyRef
			seen[ref.Name+"/"+ref.Key] = true
		}
	})
	out := make([]string, 0, len(seen))
	for ref := range seen {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// endpointOf is the entity of an external endpoint.
func endpointOf(endpoint string) inventory.EntityID {
	return inventory.CoreID(KindExternalEndpoint, "", endpoint)
}
