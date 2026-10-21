package kube

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// bindingOf is a RoleBinding of ns with count ServiceAccount subjects
// and, after them, one User.
func bindingOf(count int) *unstructured.Unstructured {
	subjects := []any{}
	for i := 0; i < count; i++ {
		subjects = append(subjects, map[string]any{
			"kind": "ServiceAccount", "name": fmt.Sprintf("sa%02d", i),
			"namespace": "ns"})
	}
	subjects = append(subjects,
		map[string]any{"kind": "User", "name": "alice"})
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rbacAPI, "kind": "RoleBinding",
		"metadata": map[string]any{"name": "b", "namespace": "ns"},
		"roleRef":  map[string]any{"kind": "ClusterRole", "name": "r"},
		"subjects": subjects,
	}}
}

func accountEntriesOf(t *testing.T, u *unstructured.Unstructured) []string {
	t.Helper()
	return strings.Fields(rbacAttrs(t, u)[AttrRBACAccounts].AsText())
}

// A binding with more subjects than kwatch keeps says so, so a reader
// does not take an account missing from the list for an unbound one.
func TestRBACBindingMarksASubjectListItCutShort(t *testing.T) {
	entries := accountEntriesOf(t, bindingOf(maxRBACItems+8))

	assert.Len(t, entries, maxRBACItems+1)
	assert.Contains(t, entries, RBACAccountsMore)
}

// Nothing is cut when every account fits; a user after them is not an
// account and does not count.
func TestRBACBindingWithAFullListHasNoMarker(t *testing.T) {
	entries := accountEntriesOf(t, bindingOf(maxRBACItems))

	assert.Len(t, entries, maxRBACItems)
	assert.NotContains(t, entries, RBACAccountsMore)
}

// The compact cache form carries the marker through.
func TestRBACTransformKeepsTheCutShortMarker(t *testing.T) {
	u := bindingOf(maxRBACItems + 8)
	out, err := rbacTransform(u.DeepCopy())
	require.NoError(t, err)

	compact := rbacAttrs(t, out.(*unstructured.Unstructured))
	assert.Equal(t, rbacAttrs(t, u), compact)
	assert.Contains(t, compact[AttrRBACAccounts].AsText(),
		RBACAccountsMore)
}
