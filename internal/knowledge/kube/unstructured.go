package kube

import (
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// AttrCustom marks entities described from unstructured objects, so the
// generic custom-resource detector knows to evaluate them.
const AttrCustom = "custom.resource"

// UnstructuredSchema describes any API object from its common fields:
// owner references, status conditions and generation. It also reads the
// references of well-known kinds (APIService backends, Gateway API
// routes) so their failures connect to the Services behind them.
type UnstructuredSchema struct {
	kind knowledge.Kind
}

// NewUnstructuredSchema builds a schema for one Kubernetes Kind.
func NewUnstructuredSchema(kubernetesKind string) UnstructuredSchema {
	return UnstructuredSchema{kind: KindFor(kubernetesKind)}
}

// Kind implements Schema.
func (s UnstructuredSchema) Kind() knowledge.Kind { return s.kind }

// RelationTypes implements Schema.
func (UnstructuredSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{
		knowledge.OwnedBy, knowledge.Serves, knowledge.RoutesTo,
		knowledge.References,
	}
}

// Describe implements Schema.
func (s UnstructuredSchema) Describe(obj any) (Description, bool) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || KindFor(u.GetKind()) != s.kind {
		return Description{}, false
	}
	attrs := map[string]knowledge.Value{
		AttrCustom:     knowledge.Bool(true),
		AttrGeneration: knowledge.Number(float64(u.GetGeneration())),
	}
	setConditions(attrs, unstructuredConditions(u))
	if ready, found, _ := unstructured.NestedBool(
		u.Object, "status", "readyToUse"); found {
		attrs[AttrReady] = knowledge.Bool(ready)
	}
	if message, found, _ := unstructured.NestedString(
		u.Object, "status", "error", "message"); found && message != "" {
		attrs[AttrMessage] = knowledge.Text(truncate(message))
	}
	rel := relations{}
	rel.add(knowledge.OwnedBy, ownerIDs(u)...)
	s.wellKnownRelations(u, rel)
	return Description{
		ID: objectID(s.kind, u), UID: string(u.GetUID()),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema. Controllers increase metadata.generation on every
// spec change and never on status updates.
func (UnstructuredSchema) Diff(old, new any) []knowledge.FieldChange {
	before, ok1 := old.(*unstructured.Unstructured)
	after, ok2 := new.(*unstructured.Unstructured)
	if !ok1 || !ok2 || before.GetGeneration() == after.GetGeneration() ||
		after.GetGeneration() == 0 {
		return nil
	}
	return []knowledge.FieldChange{{
		Path:   "spec",
		Before: "generation " + strconv.FormatInt(before.GetGeneration(), 10),
		After:  "generation " + strconv.FormatInt(after.GetGeneration(), 10),
	}}
}

func (s UnstructuredSchema) wellKnownRelations(
	u *unstructured.Unstructured, rel relations,
) {
	switch s.kind {
	case "apiservice":
		ns, _, _ := unstructured.NestedString(
			u.Object, "spec", "service", "namespace")
		name, _, _ := unstructured.NestedString(
			u.Object, "spec", "service", "name")
		rel.add(knowledge.Serves, knowledge.NewEntityID(KindService, ns, name))
	case "httproute", "grpcroute", "tlsroute", "tcproute":
		routeRelations(u, rel)
	case "volumesnapshot":
		claim, _, _ := unstructured.NestedString(u.Object, "spec", "source",
			"persistentVolumeClaimName")
		rel.add(knowledge.References,
			knowledge.NewEntityID(KindPVC, u.GetNamespace(), claim))
	}
}

// routeRelations links a Gateway API route to its backend Services and
// parent Gateways.
func routeRelations(u *unstructured.Unstructured, rel relations) {
	rules, _, _ := unstructured.NestedSlice(u.Object, "spec", "rules")
	for _, rule := range rules {
		ruleMap, _ := rule.(map[string]any)
		refs, _, _ := unstructured.NestedSlice(ruleMap, "backendRefs")
		for _, ref := range refs {
			rel.add(knowledge.RoutesTo, backendRef(u, ref, KindService))
		}
	}
	parents, _, _ := unstructured.NestedSlice(u.Object, "spec", "parentRefs")
	for _, parent := range parents {
		rel.add(knowledge.References, backendRef(u, parent, "gateway"))
	}
}

func backendRef(
	u *unstructured.Unstructured, ref any, defaultKind knowledge.Kind,
) knowledge.EntityID {
	m, _ := ref.(map[string]any)
	name, _ := m["name"].(string)
	ns, _ := m["namespace"].(string)
	if ns == "" {
		ns = u.GetNamespace()
	}
	kind := defaultKind
	if k, _ := m["kind"].(string); k != "" {
		kind = KindFor(k)
	}
	return knowledge.NewEntityID(kind, ns, name)
}

func unstructuredConditions(u *unstructured.Unstructured) []condition {
	raw, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	out := make([]condition, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := condition{
			Type: str(m, "type"), Status: str(m, "status"),
			Reason: str(m, "reason"), Message: str(m, "message"),
		}
		if at, err := time.Parse(time.RFC3339,
			str(m, "lastTransitionTime")); err == nil {
			c.Since = at
		}
		if c.Type != "" && !strings.ContainsAny(c.Type, ".") {
			out = append(out, c)
		}
	}
	return out
}

func str(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}
