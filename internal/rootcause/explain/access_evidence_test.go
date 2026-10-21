package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// quotedForbidden is a refusal naming the account that was refused.
func quotedForbidden(account string) string {
	return `configmaps is forbidden: User "system:serviceaccount:shop:` +
		account + `" cannot list resource "configmaps" in API group "" ` +
		`in the namespace "shop"`
}

// runAs gives the pods the ServiceAccount of shop that they run as.
func runAs(f *fixture, account string, pods ...inventory.EntityID) {
	id := inventory.CoreID(kube.KindAccount, "shop", account)
	f.add(id)
	for _, pod := range pods {
		f.relate(pod, inventory.References, id)
	}
}

// blames reports whether a stated cause names the root.
func blames(e Explanation, root inventory.EntityID) bool {
	for _, got := range causes(e) {
		if got == root.String() {
			return true
		}
	}
	return false
}

// TestRBACCreationIsNeverBlamed: creating a Role or binding grants
// access, it cannot be why a request was refused.
func TestRBACCreationIsNeverBlamed(t *testing.T) {
	f := newFixture(t)
	role := inventory.CoreID("role", "shop", "metrics-reader")
	f.add(role)
	f.change(role, 1)
	pods := f.workload("shop", "api", 2)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 2,
			quotedForbidden("default"))
	}
	if e := f.explain(); blames(e, role) {
		t.Fatalf("a created role was blamed: %v", causes(e))
	}
}

// TestRBACChangeNeedsThePodsOwnAccount: a refusal of another account
// says nothing about an RBAC change that may concern this pod.
func TestRBACChangeNeedsThePodsOwnAccount(t *testing.T) {
	for _, tc := range []struct {
		name, quoted string
		blamed       bool
	}{
		{"own account", "api", true},
		{"other account", "partner", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			binding := inventory.CoreID("rolebinding", "shop", "reader")
			f.add(binding)
			f.change(binding, 1, "metadata")
			pods := f.workload("shop", "api", 2)
			runAs(f, "api", pods...)
			for _, pod := range pods {
				f.fail(containerOf(pod), "CrashLoop", failingH, 2,
					quotedForbidden(tc.quoted))
			}
			if got := blames(f.explain(), binding); got != tc.blamed {
				t.Fatalf("binding blamed = %v, want %v", got, tc.blamed)
			}
		})
	}
}

// TestPolicyRestrictsNeedsAConnectionError: a policy cuts connections;
// a crash that is not about one is not its doing.
func TestPolicyRestrictsNeedsAConnectionError(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		blamed     bool
	}{
		{"refused connection", "dial tcp 10.0.0.9:5432: i/o timeout", true},
		{"code defect", rcaPanic, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			policy := inventory.CoreID(kube.KindNetworkPolicy, "shop", "deny")
			f.add(policy)
			f.change(policy, 1)
			for _, pod := range f.workload("shop", "api", 2) {
				f.links[policy] = append(f.links[policy],
					Link{Type: inventory.Selects, To: pod})
				f.fail(containerOf(pod), "CrashLoop", failingH, 2, tc.text)
			}
			if got := blames(f.explain(), policy); got != tc.blamed {
				t.Fatalf("policy blamed = %v, want %v", got, tc.blamed)
			}
		})
	}
}

// rcaPanic is a crash with a defect in the program's own code.
const rcaPanic = "panic: runtime error: invalid memory address or nil " +
	"pointer dereference"
