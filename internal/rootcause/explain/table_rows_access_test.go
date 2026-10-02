package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

// forbiddenText is how client-go reports a request RBAC refused.
const forbiddenText = `configmaps is forbidden: User ` +
	`"system:serviceaccount:shop:api" cannot list resource "configmaps" ` +
	`in API group "" in the namespace "shop"`

var accessRowCases = []rowCase{
	{row: "rbac-change", want: "rolebinding/shop/api-reader",
		build: func(f *fixture) inventory.EntityID {
			return rbacCase(f, forbiddenText)
		}},
}

// rbacCase edits a RoleBinding of shop, then crashes a workload of shop
// with text.
func rbacCase(f *fixture, text string) inventory.EntityID {
	binding := inventory.CoreID("rolebinding", "shop", "api-reader")
	f.add(binding)
	f.change(binding, 1, "metadata")
	pods := f.workload("shop", "api", 2)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 2, text)
	}
	return containerOf(pods[0])
}

// TestRBACChangeNeedsForbiddenErrors: an RBAC edit next to a crash that
// says nothing about permissions is not blamed.
func TestRBACChangeNeedsForbiddenErrors(t *testing.T) {
	f := newFixture(t)
	effect := rbacCase(f, "panic: assignment to entry in nil map")
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Root.Kind == "rolebinding" {
		t.Fatalf("an unrelated crash blamed %s", c.Root)
	}
}
