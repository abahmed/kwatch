package kube

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// A group that stands for many ServiceAccounts is kept as a wildcard
// entry, so a reader can tell an account no binding reaches from one a
// group binding grants; other groups stay out.
func TestBindingAccountsKeepWideGroups(t *testing.T) {
	binding := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rbacAPI, "kind": "ClusterRoleBinding",
		"metadata": map[string]any{"name": "b"},
		"roleRef":  map[string]any{"kind": "ClusterRole", "name": "r"},
		"subjects": []any{
			map[string]any{"kind": "Group", "name": "system:serviceaccounts"},
			map[string]any{"kind": "Group",
				"name": "system:serviceaccounts:shop"},
			map[string]any{"kind": "Group", "name": "devs"},
		},
	}}

	fields := strings.Fields(rbacAttrs(t, binding)[AttrRBACAccounts].AsText())
	sort.Strings(fields)
	assert.Equal(t, []string{"*", "shop/*"}, fields)
}

// The cache transform keeps the wide groups, never another group.
func TestRBACTransformKeepsWideGroups(t *testing.T) {
	binding := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rbacAPI, "kind": "ClusterRoleBinding",
		"metadata": map[string]any{"name": "b"},
		"roleRef":  map[string]any{"kind": "ClusterRole", "name": "r"},
		"subjects": []any{
			map[string]any{"kind": "Group", "name": "system:authenticated"},
			map[string]any{"kind": "Group", "name": "devs"},
		},
	}}
	out, err := rbacTransform(binding)
	assert.NoError(t, err)
	subjects, _, _ := unstructured.NestedSlice(
		out.(*unstructured.Unstructured).Object, "subjects")
	assert.Len(t, subjects, 1)
}
