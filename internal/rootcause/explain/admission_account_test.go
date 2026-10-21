package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const noAccount = "Error creating: pods \"api-5d8f-\" is forbidden: error " +
	"looking up service account shop/api: serviceaccount \"api\" not found"

// accountCase refuses the creates of the api ReplicaSet because its
// ServiceAccount is gone. It returns the ReplicaSet and the account.
func accountCase(f *fixture) (rs, account inventory.EntityID) {
	f.workload("shop", "api", 0)
	rs = inventory.CoreID(kube.KindReplicaSet, "shop", "api-1")
	account = inventory.CoreID(kube.KindAccount, "shop", "api")
	f.fail(rs, detection.ModeFailedCreate, failingH, 2, noAccount)
	return rs, account
}

var accountRowCases = []rowCase{
	{row: "account-missing-at-create", want: "serviceaccount/shop/api",
		build: func(f *fixture) inventory.EntityID {
			rs, _ := accountCase(f)
			return rs
		}},
}

// A refused create names the account that was not found: it is the
// cause, not the controller that was refused.
func TestAccountMissingAtCreateBlamesTheAccount(t *testing.T) {
	f := newFixture(t)
	rs, account := accountCase(f)

	requireCause(t, f.explain(), rs, account.String())
}

// An account the model holds is not missing, whatever the error says.
func TestAccountMissingAtCreateNeedsTheAccountToBeAbsent(t *testing.T) {
	f := newFixture(t)
	rs, account := accountCase(f)
	f.add(account)

	if c, ok := f.explain().CauseOf(rs); ok && c.Root == account {
		t.Fatalf("an existing account blamed: %+v", c)
	}
}

// Another account's name in the error does not blame this one.
func TestAccountMissingAtCreateNamesOnlyTheAccountInTheError(t *testing.T) {
	f := newFixture(t)
	rs, _ := accountCase(f)
	other := inventory.CoreID(kube.KindAccount, "shop", "web")

	if c, ok := f.explain().CauseOf(rs); ok && c.Root == other {
		t.Fatalf("an account the error does not name blamed: %+v", c)
	}
}
