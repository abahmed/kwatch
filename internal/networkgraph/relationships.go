package networkgraph

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func (m *Monitor) rebuild(kind string, obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
		return
	}
	if m.allowed != nil && u.GetNamespace() != "" &&
		!m.allowed(u.GetNamespace()) {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, 8)
	if kind == "gateway" {
		if class, _, _ := unstructured.NestedString(
			u.Object, "spec", "gatewayClassName",
		); class != "" {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "gatewayclass", Name: class, Type: "uses_class",
			})
		}
		targets = append(targets, secretReferences(u)...)
	} else if strings.HasSuffix(kind, "route") {
		targets = append(targets, routeParents(u)...)
		targets = append(targets, routeBackends(u)...)
	}
	ns := u.GetNamespace()
	if kind == "gatewayclass" {
		ns = ""
	}
	m.graph.ReplaceOutgoingEdges(kind, ns, u.GetName(), targets)
}

func secretReferences(u *unstructured.Unstructured) []kwcontext.EdgeTarget {
	listeners, _, _ := unstructured.NestedSlice(u.Object, "spec", "listeners")
	targets := make([]kwcontext.EdgeTarget, 0)
	for _, raw := range listeners {
		listener, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		refs, _, _ := unstructured.NestedSlice(listener, "tls", "certificateRefs")
		for _, rawRef := range refs {
			ref, ok := rawRef.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := ref["name"].(string)
			kind, _ := ref["kind"].(string)
			group, _ := ref["group"].(string)
			namespace, _ := ref["namespace"].(string)
			if namespace == "" {
				namespace = u.GetNamespace()
			}
			if name != "" &&
				(kind == "" || strings.EqualFold(kind, "Secret")) &&
				(group == "" || group == "core") {
				targets = append(targets, kwcontext.EdgeTarget{
					Kind: "secret", Namespace: namespace, Name: name,
					Type: "tls_secret",
				})
			}
		}
	}
	return targets
}

func routeParents(u *unstructured.Unstructured) []kwcontext.EdgeTarget {
	parents, _, _ := unstructured.NestedSlice(u.Object, "spec", "parentRefs")
	targets := make([]kwcontext.EdgeTarget, 0, len(parents))
	for _, raw := range parents {
		ref, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := ref["name"].(string)
		kind, _ := ref["kind"].(string)
		ns, _ := ref["namespace"].(string)
		if ns == "" {
			ns = u.GetNamespace()
		}
		if name != "" && (kind == "" || strings.EqualFold(kind, "Gateway")) {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "gateway", Namespace: ns, Name: name, Type: "routes_to",
			})
		}
	}
	return targets
}

func routeBackends(u *unstructured.Unstructured) []kwcontext.EdgeTarget {
	rules, _, _ := unstructured.NestedSlice(u.Object, "spec", "rules")
	targets := make([]kwcontext.EdgeTarget, 0)
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}
		refs, _, _ := unstructured.NestedSlice(rule, "backendRefs")
		for _, rawRef := range refs {
			ref, ok := rawRef.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := ref["name"].(string)
			kind, _ := ref["kind"].(string)
			group, _ := ref["group"].(string)
			namespace, _ := ref["namespace"].(string)
			if namespace == "" {
				namespace = u.GetNamespace()
			}
			if name != "" &&
				(kind == "" || strings.EqualFold(kind, "Service")) &&
				(group == "" || group == "core") {
				targets = append(targets, kwcontext.EdgeTarget{
					Kind: "service", Namespace: namespace, Name: name,
					Type: "routes_to",
				})
			}
		}
	}
	return targets
}
