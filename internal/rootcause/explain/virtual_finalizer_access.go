package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// roleHandlers (tier 3) are the workloads whose ServiceAccount is bound
// to a Role or ClusterRole that lets it update or patch one of the
// groups. A role that writes another group, or a wildcard, does not
// count: it says nothing about which controller serves this one.
func (v *view) roleHandlers(s finalizerSubject) []inventory.EntityID {
	accounts := v.accountGroups()
	var out []inventory.EntityID
	for _, kind := range handlerKinds {
		for _, w := range v.s.Model.Entities(kind) {
			if v.writesGroup(w, accounts, s) {
				out = append(out, w)
			}
		}
	}
	return out
}

// writesGroup reports whether an account of the workload may write one
// of the subject's groups.
func (v *view) writesGroup(
	w inventory.EntityID, accounts map[inventory.EntityID][]string,
	s finalizerSubject,
) bool {
	for _, account := range v.s.Model.Related(
		w, inventory.References, inventory.Outgoing) {
		if account.Kind != kube.KindAccount {
			continue
		}
		for _, group := range accounts[account] {
			if hasText(s.groups, group) {
				return true
			}
		}
	}
	return false
}

// accountGroups are the custom API groups each ServiceAccount may write
// through the bindings of the cluster. It is read once per view.
func (v *view) accountGroups() map[inventory.EntityID][]string {
	g := v.stuck()
	if g.accounts != nil {
		return g.accounts
	}
	g.accounts = map[inventory.EntityID][]string{}
	for _, kind := range []inventory.Kind{
		kube.KindRoleBinding, kube.KindClusterRoleBinding} {
		for _, binding := range v.s.Model.Entities(kind) {
			v.addBinding(g.accounts, binding)
		}
	}
	return g.accounts
}

// addBinding adds the groups a binding grants to the accounts it binds.
func (v *view) addBinding(
	accounts map[inventory.EntityID][]string, binding inventory.EntityID,
) {
	e, ok := v.s.Model.Entity(binding)
	if !ok {
		return
	}
	groups := v.roleGroups(attrText(e, kube.AttrRBACRole), binding.Namespace)
	if len(groups) == 0 {
		return
	}
	for _, account := range strings.Fields(attrText(e,
		kube.AttrRBACAccounts)) {
		namespace, name, _ := strings.Cut(account, "/")
		id := inventory.CoreID(kube.KindAccount, namespace, name)
		accounts[id] = append(accounts[id], groups...)
	}
}

// roleGroups are the groups a binding's roleRef ("ClusterRole/name")
// lets its holders write; a Role is looked up in the binding's namespace.
func (v *view) roleGroups(ref, namespace string) []string {
	kind, name, _ := strings.Cut(ref, "/")
	var role inventory.EntityID
	switch kind {
	case "ClusterRole":
		role = inventory.CoreID(kube.KindClusterRole, "", name)
	case "Role":
		role = inventory.CoreID(kube.KindRole, namespace, name)
	default:
		return nil
	}
	e, ok := v.s.Model.Entity(role)
	if !ok {
		return nil
	}
	return strings.Fields(attrText(e, kube.AttrRBACGroups))
}

// webhookHandlers (tier 4) are the workloads behind a webhook that
// admits one of the groups, or behind the conversion webhook of a CRD
// of one of them: the Service the webhook calls selects their pods.
func (v *view) webhookHandlers(s finalizerSubject) []inventory.EntityID {
	var out []inventory.EntityID
	for _, service := range v.webhookServices(s) {
		for _, owner := range v.backendOwners(service) {
			if slices.Contains(handlerKinds, owner.Kind) &&
				!containsID(out, owner) {
				out = append(out, owner)
			}
		}
	}
	return out
}

// webhookServices are the Services called by webhook configurations
// that cover one of the groups and by CRDs of one of them.
func (v *view) webhookServices(s finalizerSubject) []inventory.EntityID {
	var out []inventory.EntityID
	for _, from := range slices.Concat(v.hooksOf(s), v.crdsOf(s)) {
		for _, to := range v.s.Model.Related(
			from, inventory.Serves, inventory.Outgoing) {
			if to.Kind == kube.KindService && !containsID(out, to) {
				out = append(out, to)
			}
		}
	}
	return out
}

// hooksOf are the webhook configurations whose rules cover a group.
func (v *view) hooksOf(s finalizerSubject) []inventory.EntityID {
	var out []inventory.EntityID
	for _, kind := range []inventory.Kind{
		kube.KindValidatingHook, kube.KindMutatingWebhook} {
		for _, hook := range v.s.Model.Entities(kind) {
			e, _ := v.s.Model.Entity(hook)
			if anyText(s.groups, strings.Fields(attrText(e,
				kube.AttrWebhookGroups))) {
				out = append(out, hook)
			}
		}
	}
	return out
}

// crdsOf are the CRDs of one of the groups: a CRD is named
// "<plural>.<group>".
func (v *view) crdsOf(s finalizerSubject) []inventory.EntityID {
	var out []inventory.EntityID
	for _, crd := range v.s.Model.Entities(kube.KindCRD) {
		if _, group, found := strings.Cut(crd.Name, "."); found &&
			hasText(s.groups, group) {
			out = append(out, crd)
		}
	}
	return out
}

// anyText reports whether any of want is in list.
func anyText(list, want []string) bool {
	for _, item := range want {
		if hasText(list, item) {
			return true
		}
	}
	return false
}
