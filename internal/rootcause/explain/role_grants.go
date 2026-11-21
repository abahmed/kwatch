package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeRoleShort is a Role that a binding gives the refused account: no
// RBAC object changed, so its rules are the ones that do not reach the
// refused request.
const ModeRoleShort detection.Mode = "RoleShort"

// roleShortRow blames the Role bound to a refused account when
// nothing in RBAC changed.
var roleShortRow = Row{
	Name:  "role-lacks-grant",
	Cause: Side{Kind: kube.KindRole, Modes: []detection.Mode{ModeRoleShort}},
	Link:  LinkAuthorizes,
	Effect: Side{Kind: kube.KindPod, Signal: SignalForbidden,
		Modes: callerFailures},
	Prior: 0.7,
}

// boundRoles lists the Roles and ClusterRoles that bindings give the
// ServiceAccounts the pod's refusals quote. With nothing changed in
// RBAC, the refused request is one these roles do not reach.
func (v *view) boundRoles(pod inventory.EntityID) []inventory.EntityID {
	if len(v.changedRBAC()) > 0 ||
		!hasSignal(SignalForbidden, v.unitErrorText(pod)) {
		return nil
	}
	var out []inventory.EntityID
	for _, account := range quotedAccounts(v.unitErrorText(pod)) {
		for _, kind := range []inventory.Kind{
			kube.KindClusterRoleBinding, kube.KindRoleBinding} {
			for _, binding := range v.s.Model.Entities(kind) {
				if role, ok := v.boundRole(binding, account); ok &&
					!slices.Contains(out, role) {
					out = append(out, role)
				}
			}
		}
	}
	sortIDs(out)
	return out
}

// boundRole is the role a binding gives the account, when its subjects
// name it.
func (v *view) boundRole(
	binding, account inventory.EntityID,
) (inventory.EntityID, bool) {
	e, ok := v.s.Model.Entity(binding)
	if !ok {
		return inventory.EntityID{}, false
	}
	entries := strings.Fields(attrText(e, kube.AttrRBACAccounts))
	if !subjectsCover(entries, account) || subjectsCutShort(entries) {
		return inventory.EntityID{}, false
	}
	role, ok := v.roleOf(binding, attrText(e, kube.AttrRBACRole))
	if !ok {
		return inventory.EntityID{}, false
	}
	return role.ID, true
}

// roleHops lead from a refused pod to the Roles bound to its account.
func (v *view) roleHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) {
		return nil
	}
	return hopsTo(LinkAuthorizes, v.boundRoles(pod)...)
}

// roleShortModes is ModeRoleShort for a Role bound to the account the
// effect's error refuses.
func (v *view) roleShortModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkAuthorizes {
		return nil
	}
	for _, named := range v.boundRoles(effect) {
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
