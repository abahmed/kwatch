package kube

import (
	"maps"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrCustom marks entities described from unstructured objects, so the
// generic custom-resource detector knows to evaluate them.
const AttrCustom = "custom.resource"

// UnstructuredSchema describes any API object from its common fields:
// owner references, status conditions and generation. It adds generic
// links: conventional reference fields become references (uses) edges,
// a pod label selector becomes the selector attribute, and the Service
// behind a CRD conversion webhook, APIService or admission webhook
// becomes a serves (served-by) edge. It also reads the references of
// well-known kinds (Gateway API routes, volume snapshots).
type UnstructuredSchema struct {
	group string
	kind  inventory.Kind
	// spelling is the Kubernetes Kind as the API writes it.
	spelling string
}

// gatewayGroup is the Gateway API group, the default group of a route's
// parent reference.
const gatewayGroup = "gateway.networking.k8s.io"

// NewUnstructuredSchema builds a schema for one Kubernetes Kind of one API
// group ("" for the core group).
func NewUnstructuredSchema(
	apiGroup, kubernetesKind string,
) UnstructuredSchema {
	return UnstructuredSchema{
		group: GroupFor(apiGroup), kind: KindFor(kubernetesKind),
		spelling: kubernetesKind,
	}
}

// Kind implements Schema.
func (s UnstructuredSchema) Kind() inventory.Kind { return s.kind }

// RelationTypes implements Schema.
func (UnstructuredSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{
		inventory.OwnedBy, inventory.Serves, inventory.RoutesTo,
		inventory.References,
	}
}

// Describe implements Schema.
func (s UnstructuredSchema) Describe(obj any) (Description, bool) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || KindFor(u.GetKind()) != s.kind ||
		GroupForAPIVersion(u.GetAPIVersion()) != s.group {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrCustom:     inventory.Bool(true),
		AttrGeneration: inventory.Number(float64(u.GetGeneration())),
	}
	// The status cache adds this too; describing it here keeps an
	// operator's lag visible however the object was watched.
	if observed, found, _ := unstructured.NestedInt64(
		u.Object, "status", "observedGeneration"); found {
		attrs[AttrObservedGen] = inventory.Number(float64(observed))
	}
	maps.Copy(attrs, KindNameAttributes(s.spelling))
	setConditions(attrs, s.conditions(u))
	s.wellKnownStatus(u, attrs)
	if ready, found, _ := unstructured.NestedBool(
		u.Object, "status", "readyToUse"); found {
		attrs[AttrReady] = inventory.Bool(ready)
	}
	if message, found, _ := unstructured.NestedString(
		u.Object, "status", "error", "message"); found && message != "" {
		attrs[AttrMessage] = inventory.Text(evidenceText(message))
	}
	if selector := selectorAttr(u); selector != "" {
		attrs[AttrSelector] = inventory.Text(selector)
	}
	rel := relations{}
	rel.add(inventory.OwnedBy, ownerIDs(u)...)
	rel.add(inventory.References, useLinks(u)...)
	rel.add(inventory.Serves, servedByLinks(s.group, s.kind, u)...)
	s.wellKnownRelations(u, rel)
	return Description{
		ID: s.id(u), UID: string(u.GetUID()),
		Attributes: attrs, Relations: rel,
	}, true
}

func (s UnstructuredSchema) id(
	u *unstructured.Unstructured,
) inventory.EntityID {
	return inventory.NewEntityID(
		s.group, s.kind, u.GetNamespace(), u.GetName())
}

// Diff implements Schema. Controllers increase metadata.generation on every
// spec change and never on status updates.
func (UnstructuredSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*unstructured.Unstructured)
	after, ok2 := new.(*unstructured.Unstructured)
	if !ok1 || !ok2 || before.GetGeneration() == after.GetGeneration() ||
		after.GetGeneration() == 0 {
		return nil
	}
	return []inventory.FieldChange{{
		Path:   "spec",
		Before: "generation " + strconv.FormatInt(before.GetGeneration(), 10),
		After:  "generation " + strconv.FormatInt(after.GetGeneration(), 10),
	}}
}

func (s UnstructuredSchema) wellKnownRelations(
	u *unstructured.Unstructured, rel relations,
) {
	switch s.kind {
	case "httproute", "grpcroute", "tlsroute", "tcproute":
		routeRelations(u, rel)
	case "volumesnapshot":
		claim, _, _ := unstructured.NestedString(u.Object, "spec", "source",
			"persistentVolumeClaimName")
		rel.add(inventory.References,
			inventory.CoreID(KindPVC, u.GetNamespace(), claim))
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
			rel.add(inventory.RoutesTo, backendRef(u, ref, "", KindService))
		}
	}
	parents, _, _ := unstructured.NestedSlice(u.Object, "spec", "parentRefs")
	for _, parent := range parents {
		rel.add(inventory.References,
			backendRef(u, parent, gatewayGroup, "gateway"))
	}
}

// backendRef resolves a Gateway API reference. Its group and kind fields
// are optional and default per reference type.
func backendRef(
	u *unstructured.Unstructured, ref any,
	defaultGroup string, defaultKind inventory.Kind,
) inventory.EntityID {
	m, _ := ref.(map[string]any)
	name, _ := m["name"].(string)
	ns, _ := m["namespace"].(string)
	if ns == "" {
		ns = u.GetNamespace()
	}
	group, kind := defaultGroup, defaultKind
	if g, ok := m["group"].(string); ok {
		group = g
	}
	if k, _ := m["kind"].(string); k != "" {
		kind = KindFor(k)
	}
	return inventory.NewEntityID(GroupFor(group), kind, ns, name)
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
