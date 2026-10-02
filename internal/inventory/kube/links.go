package kube

import (
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Generic links read conventional reference fields from any object's
// content, so custom resources connect to the objects they use without a
// schema of their own. Extraction is pure: it names group-aware candidate
// targets and never looks at the inventory. Whether a target exists is
// decided when links are read (see Links), so a target created after its
// user still links without re-describing the user.

// KindIngressClass is the entity kind of an IngressClass.
const KindIngressClass inventory.Kind = "ingressclass"

// Bounds on the generic walk, so a large custom resource cannot produce
// an unbounded number of relations.
const (
	maxLinkDepth = 8
	maxLinks     = 64
)

// namedTarget is a conventional reference field prefix: "secret" matches
// secretName, secretRef and secretKeyRef-style fields.
type namedTarget struct {
	kind    inventory.Kind
	cluster bool
}

// namedTargets maps field prefixes to the built-in kind they name.
var namedTargets = map[string]namedTarget{
	"secret":         {kind: KindSecret},
	"secretKey":      {kind: KindSecret},
	"configMap":      {kind: KindConfigMap},
	"configMapKey":   {kind: KindConfigMap},
	"serviceAccount": {kind: KindAccount},
	"storageClass":   {kind: KindStorageClass, cluster: true},
	"ingressClass":   {kind: KindIngressClass, cluster: true},
	"priorityClass":  {kind: KindPriorityClass, cluster: true},
	"runtimeClass":   {kind: KindRuntimeClass, cluster: true},
}

// clusterKinds are built-in kinds without a namespace, so a *Ref to one
// never inherits the referencing object's namespace.
var clusterKinds = map[inventory.Kind]bool{
	KindNode: true, KindPV: true, KindNamespace: true,
	KindStorageClass: true, KindPriorityClass: true,
	KindRuntimeClass: true, KindIngressClass: true,
	"clusterrole": true, "clusterrolebinding": true,
	"customresourcedefinition": true, "apiservice": true,
}

// skippedRefKeys have dedicated relations elsewhere: Gateway API route
// references become routes-to and references edges in routeRelations.
var skippedRefKeys = map[string]bool{
	"backendRef": true, "backendRefs": true, "parentRefs": true,
}

// useLinks returns the uses (references) targets named in spec.
func useLinks(u *unstructured.Unstructured) []inventory.EntityID {
	spec, _ := u.Object["spec"].(map[string]any)
	w := linkWalker{namespace: u.GetNamespace()}
	w.walk(spec, 0)
	return w.out
}

type linkWalker struct {
	namespace string
	out       []inventory.EntityID
}

func (w *linkWalker) walk(value any, depth int) {
	if depth > maxLinkDepth || len(w.out) >= maxLinks {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			w.field(key, v[key], depth)
		}
	case []any:
		for _, item := range v {
			w.walk(item, depth+1)
		}
	}
}

func (w *linkWalker) field(key string, value any, depth int) {
	if skippedRefKeys[key] {
		return
	}
	if name, ok := value.(string); ok {
		w.add(nameFieldTarget(key, name, w.namespace))
		return
	}
	if m, ok := value.(map[string]any); ok && isRefKey(key) {
		w.add(refTarget(key, m, w.namespace))
	}
	if items, ok := value.([]any); ok && strings.HasSuffix(key, "Refs") {
		for _, item := range items {
			m, _ := item.(map[string]any)
			w.add(refTarget(strings.TrimSuffix(key, "s"), m, w.namespace))
		}
	}
	w.walk(value, depth+1)
}

func (w *linkWalker) add(id inventory.EntityID) {
	if id.Name == "" || len(w.out) >= maxLinks {
		return
	}
	for _, seen := range w.out {
		if seen == id {
			return
		}
	}
	w.out = append(w.out, id)
}

func isRefKey(key string) bool {
	return key == "ref" || strings.HasSuffix(key, "Ref")
}

// nameFieldTarget resolves a "<prefix>Name" string field, such as
// secretName, to its built-in kind.
func nameFieldTarget(key, name, namespace string) inventory.EntityID {
	prefix, ok := strings.CutSuffix(key, "Name")
	target, known := namedTargets[prefix]
	if !ok || !known {
		return inventory.EntityID{}
	}
	return namedID(target, namespace, name)
}

func namedID(target namedTarget, namespace, name string) inventory.EntityID {
	if target.cluster {
		namespace = ""
	}
	return inventory.CoreID(target.kind, namespace, name)
}

// refTarget resolves a *Ref object. An explicit kind wins and takes its
// group from apiGroup, group or apiVersion; without a kind, only a
// conventional key (secretRef, configMapKeyRef) names the target.
func refTarget(
	key string, ref map[string]any, namespace string,
) inventory.EntityID {
	name := str(ref, "name")
	if name == "" {
		return inventory.EntityID{}
	}
	if ns := str(ref, "namespace"); ns != "" {
		namespace = ns
	}
	kind := str(ref, "kind")
	if kind == "" {
		target, ok := namedTargets[strings.TrimSuffix(key, "Ref")]
		if !ok {
			return inventory.EntityID{}
		}
		return namedID(target, namespace, name)
	}
	group := refGroup(ref)
	entityKind := KindFor(kind)
	if clusterScoped(group, entityKind) {
		namespace = ""
	}
	return inventory.NewEntityID(group, entityKind, namespace, name)
}

func refGroup(ref map[string]any) string {
	if group, ok := ref["apiGroup"].(string); ok {
		return GroupFor(group)
	}
	if group, ok := ref["group"].(string); ok {
		return GroupFor(group)
	}
	return GroupForAPIVersion(str(ref, "apiVersion"))
}

// clusterScoped reports kinds without a namespace: known built-in
// cluster kinds, and by convention any kind named "Cluster*".
func clusterScoped(group string, kind inventory.Kind) bool {
	if group == "" && clusterKinds[kind] {
		return true
	}
	return strings.HasPrefix(string(kind), "cluster")
}
