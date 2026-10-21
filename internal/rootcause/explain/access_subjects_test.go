package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// bindingFixture is a RoleBinding of shop that binds accounts (as
// "namespace/name", space-separated; empty when its subjects are not
// known), changed or deleted just before the pods of api, which run as
// the account api, fail with an error that quotes quoted.
func bindingFixture(
	f *fixture, accounts string, deleted bool, quoted string,
) inventory.EntityID {
	binding := inventory.CoreID(kube.KindRoleBinding, "shop", "reader")
	attrs := map[string]inventory.Value{}
	if accounts != "" {
		attrs[kube.AttrRBACAccounts] = inventory.Text(accounts)
	}
	f.observe(binding, attrs)
	if deleted {
		f.apply(inventory.Observation{Kind: inventory.Gone, Entity: binding,
			Attributes: attrs})
		f.changes[binding] = append(f.changes[binding], inventory.Change{
			Entity: binding, At: t0.Add(time.Minute), Deleted: true})
	} else {
		f.change(binding, 1, "subjects")
	}
	pods := f.workload("shop", "api", 2)
	runAs(f, "api", pods...)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 2,
			quotedForbidden(quoted))
	}
	return binding
}

// TestRBACBindingNeedsTheRefusedAccountAsASubject: a changed or deleted
// binding is blamed only when its subjects include the account the
// refusal names (the pod's own account); one that binds another account
// cannot be why this one was refused. A binding whose subjects are not
// known stays a suspect.
func TestRBACBindingNeedsTheRefusedAccountAsASubject(t *testing.T) {
	for _, tc := range []struct {
		name, accounts string
		deleted        bool
		blamed         bool
	}{
		{"changed, binds the account", "shop/api", false, true},
		{"changed, binds another account", "shop/web", false, false},
		{"changed, binds several", "shop/api shop/web", false, true},
		{"deleted, binds the account", "shop/api", true, true},
		{"deleted, binds another account", "shop/web", true, false},
		{"deleted, subjects unknown", "", true, true},
		{"changed, subjects unknown", "", false, true},
		{"changed, binds every account", "*", false, true},
		{"changed, binds the namespace's accounts", "shop/*", false, true},
		{"deleted, binds the namespace's accounts", "shop/*", true, true},
		{"changed, binds another namespace's accounts", "lab/*", false,
			false},
		{"changed, list cut short beside another account",
			"shop/web " + kube.RBACAccountsMore, false, true},
		{"deleted, list cut short beside another account",
			"shop/web " + kube.RBACAccountsMore, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			binding := bindingFixture(f, tc.accounts, tc.deleted, "api")
			if got := blames(f.explain(), binding); got != tc.blamed {
				t.Fatalf("binding blamed = %v, want %v", got, tc.blamed)
			}
		})
	}
}

// TestRBACBindingUsesThePodsAccountWhenTheErrorNamesNone: with no account
// quoted, the pod's own account decides.
func TestRBACBindingUsesThePodsAccountWhenTheErrorNamesNone(t *testing.T) {
	f := newFixture(t)
	binding := bindingFixture(f, "shop/web", false, "api")
	for _, pod := range f.model.Entities(kube.KindPod) {
		f.findings[containerOf(pod)] = nil
		f.fail(containerOf(pod), "CrashLoop", failingH, 2,
			`configmaps is forbidden: cannot list resource "configmaps"`)
	}
	if blames(f.explain(), binding) {
		t.Fatal("a binding of another account was blamed")
	}
}
