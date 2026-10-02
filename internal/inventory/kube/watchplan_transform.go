package kube

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// lastAppliedAnnotation holds a full copy of the object; it is dropped
// from dynamic caches.
const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// maxFlatSpecEntries bounds the flat spec maps a status cache keeps.
const maxFlatSpecEntries = 16

// specRules are spec fields a status cache keeps with a pruning
// function; every other field is kept only when it is a scalar, a flat
// map of scalars, or named like a reference (see keepSpecField).
var specRules = map[string]func(any) any{
	// Gateway API route rules: only their backend references.
	"rules": pruneRules,
}

// statusTransform keeps an unstructured object's metadata, status and
// the reference-bearing spec subset, so status caches stay small while
// relations (owners, backends, parents, claims) still resolve.
func statusTransform(obj any) (any, error) {
	obj, err := TrimToLatestManager(obj)
	if err != nil {
		return obj, err
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj, nil
	}
	out := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": u.GetAPIVersion(), "kind": u.GetKind(),
	}}
	if metadata, ok := u.Object["metadata"].(map[string]any); ok {
		out.Object["metadata"] = metadata
	}
	dropLastApplied(out)
	if status, ok := u.Object["status"]; ok {
		out.Object["status"] = status
	}
	if spec, ok := u.Object["spec"].(map[string]any); ok {
		if subset := specSubset(spec); len(subset) > 0 {
			out.Object["spec"] = subset
		}
	}
	return out, nil
}

// metadataTransform trims a metadata-only object.
func metadataTransform(obj any) (any, error) {
	obj, err := TrimToLatestManager(obj)
	if err != nil {
		return obj, err
	}
	if m, ok := obj.(*metav1.PartialObjectMetadata); ok {
		dropLastApplied(m)
	}
	return obj, nil
}

func dropLastApplied(obj metav1.Object) {
	annotations := obj.GetAnnotations()
	if _, ok := annotations[lastAppliedAnnotation]; !ok {
		return
	}
	kept := make(map[string]string, len(annotations)-1)
	for k, v := range annotations {
		if k != lastAppliedAnnotation {
			kept[k] = v
		}
	}
	obj.SetAnnotations(kept)
}

func specSubset(spec map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range spec {
		if secretSpecKey(key) {
			continue
		}
		if prune, ok := specRules[key]; ok {
			if pruned := prune(value); pruned != nil {
				out[key] = pruned
			}
			continue
		}
		if kept, ok := keepSpecField(key, value); ok {
			out[key] = kept
		}
	}
	return out
}

// keepSpecField keeps scalars, small flat maps (an APIService's service,
// a snapshot's source) and fields named like references or selectors.
// It returns the value to keep: a flat map loses its secret-named
// entries.
func keepSpecField(key string, value any) (any, bool) {
	if isScalar(value) {
		return value, true
	}
	for _, suffix := range []string{"Ref", "Refs", "Selector", "selector"} {
		if strings.HasSuffix(key, suffix) {
			return value, true
		}
	}
	m, ok := value.(map[string]any)
	if !ok || len(m) > maxFlatSpecEntries {
		return nil, false
	}
	kept := make(map[string]any, len(m))
	for k, v := range m {
		if !isScalar(v) {
			return nil, false
		}
		if !secretSpecKey(k) {
			kept[k] = v
		}
	}
	return kept, true
}

// secretSpecWords is the secret-key vocabulary of internal/redact, with
// the camelCase spellings custom resources use (apiKey, privateKey,
// secureJsonData). Keys are compared lower-cased, without "_" and "-".
var secretSpecWords = []string{
	"password", "passwd", "passphrase", "token", "secret", "apikey",
	"accesskey", "privatekey", "credential", "securejsondata",
}

// referenceSuffixes name a field that only points at a secret, such as
// secretRef or tokenSecretName. The Secret itself is watched with its
// values hashed, so the reference is kept: relations need it.
var referenceSuffixes = []string{
	"ref", "refs", "name", "names", "namespace",
}

// secretSpecKey reports whether a spec field is named like a credential.
// Status caches drop such fields: a custom resource can carry a password
// or token inline in its spec.
func secretSpecKey(key string) bool {
	lower := strings.ToLower(key)
	for _, suffix := range referenceSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	compact := strings.NewReplacer("_", "", "-", "").Replace(lower)
	for _, word := range secretSpecWords {
		if strings.Contains(compact, word) {
			return true
		}
	}
	return false
}

func isScalar(value any) bool {
	switch v := value.(type) {
	case string:
		return len(v) <= maxMessageLength
	case bool, int64, float64, int32, int:
		return true
	}
	return false
}

func pruneRules(value any) any {
	rules, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(rules))
	for _, rule := range rules {
		m, _ := rule.(map[string]any)
		refs, ok := m["backendRefs"]
		if !ok {
			out = append(out, map[string]any{})
			continue
		}
		out = append(out, map[string]any{"backendRefs": refs})
	}
	return out
}
