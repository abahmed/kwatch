package explain

import (
	"regexp"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// accountQuote matches the caller a "forbidden" error names:
// User "system:serviceaccount:<namespace>:<name>".
var accountQuote = regexp.MustCompile(
	`system:serviceaccount:([a-z0-9][a-z0-9.-]*):([a-z0-9][a-z0-9.-]*)`)

// refusedOtherAccount reports a pod whose errors name only
// ServiceAccounts other than the ones it runs as. Such a refusal is
// someone else's, so no RBAC change can be tied to this pod by it. A
// pod with no recorded account, or errors that name none, says nothing.
func (v *view) refusedOtherAccount(pod inventory.EntityID) bool {
	own := v.accountsOf(pod)
	quoted := quotedAccounts(v.unitErrorText(pod))
	if len(own) == 0 || len(quoted) == 0 {
		return false
	}
	for _, account := range quoted {
		if slices.Contains(own, account) {
			return false
		}
	}
	return true
}

// accountsOf lists the ServiceAccounts a pod references.
func (v *view) accountsOf(pod inventory.EntityID) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range v.s.Model.Related(
		pod, inventory.References, inventory.Outgoing,
	) {
		if id.Kind == kube.KindAccount {
			out = append(out, id)
		}
	}
	return out
}

// quotedAccounts lists the ServiceAccounts named in text.
func quotedAccounts(text string) []inventory.EntityID {
	var out []inventory.EntityID
	for _, m := range accountQuote.FindAllStringSubmatch(
		strings.ToLower(text), -1) {
		out = append(out, inventory.CoreID(kube.KindAccount, m[1], m[2]))
	}
	return out
}

// refusedAccounts are the ServiceAccounts a pod's refusal concerns: the
// ones its errors name, or else the ones it runs as.
func (v *view) refusedAccounts(pod inventory.EntityID) []inventory.EntityID {
	if quoted := quotedAccounts(v.unitErrorText(pod)); len(quoted) > 0 {
		return quoted
	}
	return v.accountsOf(pod)
}

// bindsOtherAccounts reports an RBAC binding whose subjects are known
// and include none of accounts: changing or deleting it cannot be why
// they were refused. A binding whose subjects are not known (it was
// deleted while kwatch was down, it binds only users and groups, or its
// list was cut short) and any other RBAC object stay suspects. Only
// ServiceAccount subjects are kept, plus the groups that stand for many
// of them, so a binding that lists such a group binds every account it
// covers.
func (v *view) bindsOtherAccounts(
	binding inventory.EntityID, accounts []inventory.EntityID,
) bool {
	if binding.Kind != kube.KindRoleBinding &&
		binding.Kind != kube.KindClusterRoleBinding {
		return false
	}
	subjects := v.bindingAccounts(binding)
	if len(subjects) == 0 || len(accounts) == 0 ||
		subjectsCutShort(subjects) {
		return false
	}
	for _, account := range accounts {
		if subjectsCover(subjects, account) {
			return false
		}
	}
	return true
}

// subjectsCover reports whether a binding's subject entries name the
// account: by name, or through a group of the accounts of its namespace
// or of every namespace.
func subjectsCover(entries []string, account inventory.EntityID) bool {
	exact := account.Namespace + "/" + account.Name
	wide := account.Namespace + "/*"
	for _, entry := range entries {
		if entry == exact || entry == wide || entry == "*" {
			return true
		}
	}
	return false
}

// subjectsCutShort reports a subject list that kwatch did not keep
// whole: an account may be bound in the part it dropped.
func subjectsCutShort(entries []string) bool {
	return slices.Contains(entries, kube.RBACAccountsMore)
}

// goneAttributes is the part of the inventory that keeps what a source
// handed over when it deleted an entity. The production model has it;
// small test models may not.
type goneAttributes interface {
	GoneAttribute(id inventory.EntityID, name string) (inventory.Value, bool)
}

// bindingAccounts are the ServiceAccounts a binding binds, as
// "namespace/name": read from the binding, or from what its deletion
// handed over.
func (v *view) bindingAccounts(binding inventory.EntityID) []string {
	if e, ok := v.s.Model.Entity(binding); ok {
		return strings.Fields(attrText(e, kube.AttrRBACAccounts))
	}
	gone, ok := v.s.Model.(goneAttributes)
	if !ok {
		return nil
	}
	value, ok := gone.GoneAttribute(binding, kube.AttrRBACAccounts)
	if !ok {
		return nil
	}
	return strings.Fields(value.AsText())
}
