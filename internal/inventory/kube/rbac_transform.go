package kube

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// rbacResources are the built-in RBAC kinds the dynamic source keeps in
// a compact form instead of as metadata only: the root-cause engine
// needs the API groups their rules write (see AttrRBACGroups).
var rbacResources = map[Resource]bool{
	{Group: "rbac.authorization.k8s.io", Name: "roles"}:               true,
	{Group: "rbac.authorization.k8s.io", Name: "rolebindings"}:        true,
	{Group: "rbac.authorization.k8s.io", Name: "clusterroles"}:        true,
	{Group: "rbac.authorization.k8s.io", Name: "clusterrolebindings"}: true,
}

// isRBACResource reports a resource watched through rbacTransform.
func isRBACResource(gvr schema.GroupVersionResource) bool {
	return rbacResources[Resource{Group: gvr.Group, Name: gvr.Resource}]
}

// rbacTransform keeps an RBAC object's metadata and only what
// setRBACAttributes reads: the rules that change an object, the role a
// binding grants and the ServiceAccounts it binds. Resource names, other
// subjects and the rest of the rules are dropped, so a cache holds a few
// bytes per role however large its rules are.
func rbacTransform(obj any) (any, error) {
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
	if rules := writeRules(u); len(rules) > 0 {
		out.Object["rules"] = rules
	}
	if ref, ok := u.Object["roleRef"].(map[string]any); ok {
		out.Object["roleRef"] = map[string]any{
			"kind": str(ref, "kind"), "name": str(ref, "name")}
	}
	if subjects := accountSubjects(u); len(subjects) > 0 {
		out.Object["subjects"] = subjects
	}
	return out, nil
}

// writeRules are the rules that let a holder update or patch, reduced
// to their API groups and the verbs that matter.
func writeRules(u *unstructured.Unstructured) []any {
	rules, _, _ := unstructured.NestedSlice(u.Object, "rules")
	var out []any
	for _, item := range rules {
		rule, _ := item.(map[string]any)
		if !hasWriteVerb(stringList(rule["verbs"])) {
			continue
		}
		groups := make([]any, 0)
		for _, group := range stringList(rule["apiGroups"]) {
			if customGroup(group) {
				groups = append(groups, group)
			}
		}
		if len(groups) > 0 {
			out = append(out, map[string]any{
				"apiGroups": groups, "verbs": []any{"update"}})
		}
	}
	return out
}

// accountSubjects are the ServiceAccount subjects of a binding, and the
// groups that stand for many of them, at most maxRBACItems. When more
// would follow, a Group named RBACAccountsMore says so.
func accountSubjects(u *unstructured.Unstructured) []any {
	subjects, _, _ := unstructured.NestedSlice(u.Object, "subjects")
	var out []any
	for _, item := range subjects {
		subject, _ := item.(map[string]any)
		if _, ok := accountEntry(subject, u.GetNamespace()); !ok {
			continue
		}
		if len(out) >= maxRBACItems {
			return append(out, map[string]any{
				"kind": "Group", "name": RBACAccountsMore})
		}
		if _, wide := wideAccounts(subject); wide {
			out = append(out, map[string]any{
				"kind": "Group", "name": str(subject, "name")})
			continue
		}
		out = append(out, map[string]any{
			"kind": "ServiceAccount", "name": str(subject, "name"),
			"namespace": str(subject, "namespace")})
	}
	return out
}
