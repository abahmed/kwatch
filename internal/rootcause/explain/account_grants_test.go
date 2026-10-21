package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const noGrantText = "pods is forbidden: User " +
	"\"system:serviceaccount:shop:api\" cannot list resource \"pods\" " +
	"in API group \"\" in the namespace \"shop\""

// grantCase crash-loops api on a forbidden error naming its own
// ServiceAccount. It returns the failing container and the account.
func grantCase(f *fixture, text string) (effect, account inventory.EntityID) {
	pod := f.workload("shop", "api", 1)[0]
	account = inventory.CoreID(kube.KindAccount, "shop", "api")
	f.add(account)
	f.relate(pod, inventory.References, account)
	f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2, text)
	return containerOf(pod), account
}

// A refusal for the pod's own account, with no binding naming it, is
// the account's: nothing is granted to it.
func TestAccountWithNoBindingIsTheCauseOfItsRefusals(t *testing.T) {
	f := newFixture(t)
	effect, account := grantCase(f, noGrantText)

	c := requireCause(t, f.explain(), effect, account.String())
	if c.Row != "account-has-no-grant" {
		t.Fatalf("row = %s, want account-has-no-grant", c.Row)
	}
}

// A binding that names the account may grant something else: the
// refusal is not "no grant".
func TestAccountWithABindingIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	effect, account := grantCase(f, noGrantText)
	f.writeAccess(account, "gadgets.acme.dev")

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == account {
		t.Fatalf("a bound account blamed for no grant: %+v", c)
	}
}

// A group binding that reaches every account of the namespace counts.
func TestAccountReachedByAGroupBindingIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	effect, account := grantCase(f, noGrantText)
	binding := inventory.CoreID(kube.KindClusterRoleBinding, "", "all")
	f.add(binding)
	f.setAttrs(binding, attrs{kube.AttrRBACRole: inventory.Text(
		"ClusterRole/view"), kube.AttrRBACAccounts: inventory.Text(
		"shop/*")})

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == account {
		t.Fatalf("an account a group binding reaches blamed: %+v", c)
	}
}

// A binding whose subject list was cut short may name the account in
// the part kwatch did not keep: that is not proof of "no grant".
func TestAccountWithACutShortBindingIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	effect, account := grantCase(f, noGrantText)
	binding := inventory.CoreID(kube.KindClusterRoleBinding, "", "many")
	f.add(binding)
	f.setAttrs(binding, attrs{kube.AttrRBACRole: inventory.Text(
		"ClusterRole/view"), kube.AttrRBACAccounts: inventory.Text(
		"lab/a " + kube.RBACAccountsMore)})

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == account {
		t.Fatalf("blamed beside a cut-short binding: %+v", c)
	}
}

// Another user in the error is not the pod's own account.
func TestAccountNoGrantNamesOnlyThePodsOwnAccount(t *testing.T) {
	f := newFixture(t)
	effect, account := grantCase(f, "User \"partner-bot\" cannot list "+
		"resource \"pods\" in API group \"\"")

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == account {
		t.Fatalf("an account the error does not name blamed: %+v", c)
	}
}

// Bindings that kwatch does not watch cannot show that none names the
// account: the cause stays unknown.
func TestAccountNoGrantNeedsVisibleBindings(t *testing.T) {
	f := newFixture(t)
	effect, account := grantCase(f, noGrantText)
	s := f.snapshot()
	s.Synced = func(k inventory.Kind) bool {
		return k != kube.KindRoleBinding &&
			k != kube.KindClusterRoleBinding
	}

	if c, ok := Explain(s).CauseOf(effect); ok && c.Root == account {
		t.Fatalf("blamed with the bindings unwatched: %+v", c)
	}
}

var grantRowCases = []rowCase{
	{row: "account-has-no-grant", want: "serviceaccount/shop/api",
		build: func(f *fixture) inventory.EntityID {
			effect, _ := grantCase(f, noGrantText)
			return effect
		}},
}
