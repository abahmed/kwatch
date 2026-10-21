package kube

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/inventory"
)

const rbacAPI = "rbac.authorization.k8s.io/v1"

func rule(verbs []any, groups ...any) map[string]any {
	return map[string]any{"verbs": verbs, "apiGroups": groups,
		"resources":     []any{"widgets", "widgets/finalizers"},
		"resourceNames": []any{"secret-name"}}
}

func clusterRole() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rbacAPI, "kind": "ClusterRole",
		"metadata": map[string]any{"name": "widget-ctl"},
		"rules": []any{
			rule([]any{"get", "list", "watch"}, "readonly.example.com"),
			rule([]any{"update", "patch"}, "widgets.example.com", ""),
			rule([]any{"*"}, "*"),
			rule([]any{"patch"}, "apps", "gadgets.example.com"),
		},
	}}
}

func roleBinding() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rbacAPI, "kind": "RoleBinding",
		"metadata": map[string]any{"name": "b", "namespace": "ops"},
		"roleRef": map[string]any{"kind": "ClusterRole",
			"name": "widget-ctl", "apiGroup": "rbac.authorization.k8s.io"},
		"subjects": []any{
			map[string]any{"kind": "ServiceAccount", "name": "ctl",
				"namespace": "widgets"},
			map[string]any{"kind": "ServiceAccount", "name": "local"},
			map[string]any{"kind": "User", "name": "alice"},
			map[string]any{"kind": "Group", "name": "devs"},
		},
	}}
}

func rbacAttrs(t *testing.T, u *unstructured.Unstructured,
) map[string]inventory.Value {
	t.Helper()
	d, ok := NewUnstructuredSchema("rbac.authorization.k8s.io",
		u.GetKind()).Describe(u)
	require.True(t, ok)
	return d.Attributes
}

// Only the custom groups a role lets its holder update or patch are kept:
// not read-only grants, not wildcards, not groups the API server serves.
func TestRBACRoleKeepsTheCustomGroupsItCanWrite(t *testing.T) {
	attrs := rbacAttrs(t, clusterRole())

	assert.Equal(t, "gadgets.example.com widgets.example.com",
		attrs[AttrRBACGroups].AsText())
}

func TestRBACBindingKeepsItsRoleAndServiceAccounts(t *testing.T) {
	attrs := rbacAttrs(t, roleBinding())

	assert.Equal(t, "ClusterRole/widget-ctl", attrs[AttrRBACRole].AsText())
	assert.Equal(t, "ops/local widgets/ctl",
		attrs[AttrRBACAccounts].AsText())
}

// The compact cache form describes exactly as the full object does, and
// holds no resource, resource name or non-account subject.
func TestRBACTransformKeepsWhatTheAttributesRead(t *testing.T) {
	for _, u := range []*unstructured.Unstructured{
		clusterRole(), roleBinding()} {
		t.Run(u.GetKind(), func(t *testing.T) {
			out, err := rbacTransform(u.DeepCopy())
			require.NoError(t, err)
			compact := out.(*unstructured.Unstructured)

			assert.Equal(t, rbacAttrs(t, u), rbacAttrs(t, compact))
			assert.NotContains(t, compact.Object, "aggregationRule")
			raw := compact.UnstructuredContent()
			assert.NotContains(t, raw, "resources")
			assert.NotContains(t, string(mustJSON(t, raw)), "secret-name")
			assert.NotContains(t, string(mustJSON(t, raw)), "alice")
		})
	}
}

func TestRBACKindsAreWatchedCompactly(t *testing.T) {
	for _, name := range []string{"roles", "rolebindings", "clusterroles",
		"clusterrolebindings"} {
		gvr := schema.GroupVersionResource{
			Group: "rbac.authorization.k8s.io", Version: "v1",
			Resource: name}
		p, ok := planResource(gvr, "X", false, typedResources())
		require.True(t, ok, name)
		assert.Equal(t, WatchStatus, p.mode, name)
		assert.True(t, isRBACResource(gvr), name)
	}
}

// A custom kind that happens to be called Role is not read as RBAC.
func TestRBACAttributesIgnoreCustomKindsNamedLikeRoles(t *testing.T) {
	u := clusterRole()
	u.SetAPIVersion("example.com/v1")
	d, ok := NewUnstructuredSchema("example.com", "ClusterRole").Describe(u)
	require.True(t, ok)

	assert.NotContains(t, d.Attributes, AttrRBACGroups)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
