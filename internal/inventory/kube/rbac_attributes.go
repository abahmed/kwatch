package kube

import (
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Entity kinds of the RBAC objects. The dynamic source watches them for
// the one fact the root-cause engine reads from them: which API groups a
// ServiceAccount may write.
const (
	KindRole               inventory.Kind = "role"
	KindClusterRole        inventory.Kind = "clusterrole"
	KindRoleBinding        inventory.Kind = "rolebinding"
	KindClusterRoleBinding inventory.Kind = "clusterrolebinding"
)

// Attributes of the RBAC objects. They are compact on purpose: no
// resource, verb or resource name is kept beyond what these say.
const (
	// AttrRBACGroups, on a Role or ClusterRole, lists the custom API
	// groups its rules let a holder update or patch, space-separated and
	// sorted. Wildcard groups and built-in groups are left out: they
	// say nothing about which controller serves a custom group.
	AttrRBACGroups = "rbac.groups"
	// AttrRBACRole, on a binding, is the role it grants, as
	// "ClusterRole/name" or "Role/name".
	AttrRBACRole = "rbac.role"
	// AttrRBACAccounts, on a binding, lists the ServiceAccounts it
	// binds as "namespace/name", space-separated; "*" and
	// "namespace/*" stand for a group of every account.
	AttrRBACAccounts = "rbac.accounts"
	// RBACAccountsMore is an entry of an AttrRBACAccounts list that
	// kwatch cut short: the binding names more subjects than the list
	// keeps, so no account can be shown to be missing from it.
	RBACAccountsMore = "+more"
)

// maxRBACItems bounds the groups and accounts one object keeps.
const maxRBACItems = 32

// changeVerbs are the verbs that let a holder change an object, and so
// remove its finalizers.
var changeVerbs = map[string]bool{"update": true, "patch": true, "*": true}

// rbacKinds are the kinds setRBACAttributes reads.
var rbacKinds = map[inventory.Kind]bool{
	KindRole: true, KindClusterRole: true,
	KindRoleBinding: true, KindClusterRoleBinding: true,
}

// setRBACAttributes adds the compact RBAC attributes of a built-in role
// or binding. Any other object is left alone.
func setRBACAttributes(
	group string, kind inventory.Kind, u *unstructured.Unstructured,
	attrs map[string]inventory.Value,
) {
	if group != "" || !rbacKinds[kind] {
		return
	}
	switch kind {
	case KindRole, KindClusterRole:
		if groups := writableGroups(u); len(groups) > 0 {
			attrs[AttrRBACGroups] = inventory.Text(
				strings.Join(groups, " "))
		}
	default:
		setBindingAttributes(u, attrs)
	}
}

// writableGroups are the custom API groups some rule of the role lets a
// holder update or patch.
func writableGroups(u *unstructured.Unstructured) []string {
	rules, _, _ := unstructured.NestedSlice(u.Object, "rules")
	seen := map[string]bool{}
	var out []string
	for _, item := range rules {
		rule, _ := item.(map[string]any)
		if !hasWriteVerb(stringList(rule["verbs"])) {
			continue
		}
		for _, group := range stringList(rule["apiGroups"]) {
			if !customGroup(group) || seen[group] {
				continue
			}
			seen[group] = true
			out = append(out, group)
		}
	}
	sort.Strings(out)
	if len(out) > maxRBACItems {
		out = out[:maxRBACItems]
	}
	return out
}

// customGroup reports an API group a controller of its own serves: not a
// wildcard and not one the API server serves itself.
func customGroup(group string) bool {
	return group != "*" && group != "" && GroupFor(group) != ""
}

func hasWriteVerb(verbs []string) bool {
	for _, verb := range verbs {
		if changeVerbs[verb] {
			return true
		}
	}
	return false
}

// setBindingAttributes records the role a binding grants and the
// ServiceAccounts that receive it.
func setBindingAttributes(
	u *unstructured.Unstructured, attrs map[string]inventory.Value,
) {
	kind, _, _ := unstructured.NestedString(u.Object, "roleRef", "kind")
	name, _, _ := unstructured.NestedString(u.Object, "roleRef", "name")
	if kind != "" && name != "" {
		attrs[AttrRBACRole] = inventory.Text(kind + "/" + name)
	}
	if accounts := accountEntries(u); len(accounts) > 0 {
		sort.Strings(accounts)
		attrs[AttrRBACAccounts] = inventory.Text(
			strings.Join(accounts, " "))
	}
}

// accountEntries are the entries of AttrRBACAccounts for a binding's
// subjects: at most maxRBACItems, then RBACAccountsMore when another
// account subject would not fit.
func accountEntries(u *unstructured.Unstructured) []string {
	subjects, _, _ := unstructured.NestedSlice(u.Object, "subjects")
	var out []string
	for _, item := range subjects {
		subject, _ := item.(map[string]any)
		entry, ok := accountEntry(subject, u.GetNamespace())
		switch {
		case !ok:
			continue
		case entry == RBACAccountsMore || len(out) >= maxRBACItems:
			return append(out, RBACAccountsMore)
		}
		out = append(out, entry)
	}
	return out
}

// accountEntry is the entry one subject adds to AttrRBACAccounts, if it
// adds one: a ServiceAccount, or a group that stands for many of them.
func accountEntry(
	subject map[string]any, namespace string,
) (string, bool) {
	if wide, ok := wideAccounts(subject); ok {
		return wide, true
	}
	name := str(subject, "name")
	if str(subject, "kind") != "ServiceAccount" || name == "" {
		return "", false
	}
	if own := str(subject, "namespace"); own != "" {
		namespace = own
	}
	return namespace + "/" + name, true
}

// wideAccounts is the entry a Group subject that stands for many
// ServiceAccounts adds to AttrRBACAccounts: "*" for every account (the
// system:serviceaccounts and system:authenticated groups) and
// "namespace/*" for every account of one namespace. Readers use it to
// tell an account no binding reaches from one a group binding does. A
// group named RBACAccountsMore is the cache's stand-in for the subjects
// it dropped (see accountSubjects).
func wideAccounts(subject map[string]any) (string, bool) {
	if str(subject, "kind") != "Group" {
		return "", false
	}
	const accountsGroup = "system:serviceaccounts"
	name := str(subject, "name")
	switch {
	case name == RBACAccountsMore:
		return RBACAccountsMore, true
	case name == accountsGroup || name == "system:authenticated":
		return "*", true
	case strings.HasPrefix(name, accountsGroup+":"):
		return strings.TrimPrefix(name, accountsGroup+":") + "/*", true
	}
	return "", false
}

// stringList reads a list of strings from decoded JSON.
func stringList(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
