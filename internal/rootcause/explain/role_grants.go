package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeRoleShort is a Role named for the account that was refused: no
// RBAC object changed, and kwatch reads only metadata, so the Role of
// that account is the one whose rules do not reach the refused request.
const ModeRoleShort detection.Mode = "RoleShort"

// roleShortRow blames the Role named for a refused account when
// nothing in RBAC changed.
var roleShortRow = Row{
	Name:  "role-lacks-grant",
	Cause: Side{Kind: kube.KindRole, Modes: []detection.Mode{ModeRoleShort}},
	Link:  LinkAuthorizes,
	Effect: Side{Kind: kube.KindPod, Signal: SignalForbidden,
		Modes: callerFailures},
	Prior: 0.7,
}

// namedRoles lists the Roles of the pod's namespace whose names carry
// the name of a ServiceAccount its refusals quote.
func (v *view) namedRoles(pod inventory.EntityID) []inventory.EntityID {
	if len(v.changedRBAC()) > 0 ||
		!hasSignal(SignalForbidden, v.unitErrorText(pod)) {
		return nil
	}
	var out []inventory.EntityID
	for _, account := range quotedAccounts(v.unitErrorText(pod)) {
		word, ok := nameWord(account.Name)
		if !ok || account.Namespace != pod.Namespace {
			continue
		}
		for _, id := range v.s.Model.EntitiesIn(
			kube.KindRole, pod.Namespace) {
			if strings.Contains(strings.ToLower(id.Name), word) {
				out = append(out, id)
			}
		}
	}
	return out
}

// roleHops lead from a refused pod to the Roles named for its account.
func (v *view) roleHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) {
		return nil
	}
	return hopsTo(LinkAuthorizes, v.namedRoles(pod)...)
}

// roleShortModes is ModeRoleShort for a Role named for the account the
// effect's error refuses.
func (v *view) roleShortModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkAuthorizes {
		return nil
	}
	for _, named := range v.namedRoles(effect) {
		if named == id {
			return pseudoFailing(ModeRoleShort)
		}
	}
	return nil
}

// accountReplacedRow blames a ServiceAccount that was deleted and
// created again for the refusals its running pods get: their tokens
// name the old identity.
var accountReplacedRow = Row{
	Name: "account-replaced",
	Cause: Side{Kind: kube.KindAccount,
		Modes: []detection.Mode{ModeChanged, ModeCreated, ModeDeleted}},
	Link: LinkUses,
	Effect: Side{Kind: kube.KindPod, Signal: SignalUnauthorized,
		Modes: callerFailures},
	Prior: 0.75,
}

// unauthorizedHops lead from a pod refused with 401 to the
// ServiceAccounts it runs as: their identity may have been replaced.
func (v *view) unauthorizedHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) ||
		!hasSignal(SignalUnauthorized, v.unitErrorText(pod)) {
		return nil
	}
	return hopsTo(LinkUses, v.accountsOf(pod)...)
}
