package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeNoGrant is a ServiceAccount that no binding names: the API server
// refused it a request, and kwatch sees no Role or ClusterRole bound to
// it. It is a pseudo mode read from the refused pod's own error.
const ModeNoGrant detection.Mode = "NoGrant"

// forbiddenUser finds the ServiceAccount an API refusal names as the
// user, in lower-case text: User "system:serviceaccount:shop:api".
var forbiddenUser = regexp.MustCompile(
	`system:serviceaccount:([a-z0-9.-]+):([a-z0-9.-]+)`)

// accountNoGrantRow blames a ServiceAccount no binding names for the
// refusals its own pods get.
var accountNoGrantRow = Row{
	Name: "account-has-no-grant",
	Cause: Side{Kind: kube.KindAccount,
		Modes: []detection.Mode{ModeNoGrant}},
	Link: LinkUses,
	Effect: Side{Kind: kube.KindPod, Modes: callerFailures,
		Signal: SignalForbidden},
	Prior: 0.7,
}

// noGrantModes is ModeNoGrant for the account the effect's own error
// names as its user, when kwatch can see every binding and none names
// the account. It says nothing, so the cause stays unknown, when the
// bindings are not watched or one changed in the window: a binding that
// was just deleted is the rbac-change row's cause.
func (v *view) noGrantModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkUses || id.Kind != kube.KindAccount {
		return nil
	}
	named, ok := v.forbiddenAccount(effect)
	if !ok || named != id || !v.bindingsVisible() ||
		len(v.changedRBAC()) > 0 || v.accountBound(id) {
		return nil
	}
	return pseudoFailing(ModeNoGrant)
}

// forbiddenAccount is the ServiceAccount the effect's error names as
// the user a request was refused for.
func (v *view) forbiddenAccount(
	effect inventory.EntityID,
) (inventory.EntityID, bool) {
	m := forbiddenUser.FindStringSubmatch(
		strings.ToLower(v.errorText(effect)))
	if m == nil {
		return inventory.EntityID{}, false
	}
	return inventory.CoreID(kube.KindAccount, m[1], m[2]), true
}

// bindingsVisible reports that both kinds of binding are watched.
func (v *view) bindingsVisible() bool {
	return v.s.synced(kube.KindClusterRoleBinding) &&
		v.s.synced(kube.KindRoleBinding)
}

// accountBound reports whether any binding names the account, by name
// or through a group of accounts. A binding whose subject list was cut
// short may name it too, so it counts: kwatch cannot show that none does.
func (v *view) accountBound(account inventory.EntityID) bool {
	for _, kind := range []inventory.Kind{
		kube.KindClusterRoleBinding, kube.KindRoleBinding} {
		for _, id := range v.s.Model.Entities(kind) {
			e, ok := v.s.Model.Entity(id)
			if !ok {
				continue
			}
			entries := strings.Fields(attrText(e, kube.AttrRBACAccounts))
			if subjectsCover(entries, account) || subjectsCutShort(entries) {
				return true
			}
		}
	}
	return false
}
