package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A custom resource names its controller in its field manager only when
// that manager is the workload's name. Any other manager ("manager",
// the scaffolding default) says nothing, so the controller is also found
// by what the cluster grants: the workload whose ServiceAccount may
// update the resource's API group is the one that writes it.

// grantedControllers lists the controller workloads of a custom
// resource that stopped being reconciled: those whose pods run as a
// ServiceAccount that a binding gives write access to the resource's
// group.
func (v *view) grantedControllers(
	id inventory.EntityID,
) []inventory.EntityID {
	if id.Group == "" || !v.hasMode(id, []detection.Mode{
		detection.ModeNotReconciling, detection.ModeCondition}) {
		return nil
	}
	seen := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, account := range v.groupWriters()[id.Group] {
		for _, pod := range v.s.Model.Related(account,
			inventory.References, inventory.Incoming) {
			top := rootcause.TopOwner(v.s.Model, pod)
			if pod.Kind != kube.KindPod || seen[top] ||
				!isHandlerKind(top.Kind) {
				continue
			}
			seen[top] = true
			v.gate(id, top)
			out = append(out, top)
		}
	}
	sortIDs(out)
	return out
}

// isHandlerKind reports a workload kind that may run a controller.
func isHandlerKind(kind inventory.Kind) bool {
	for _, k := range handlerKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// groupWriters maps an API group to the ServiceAccounts the cluster's
// bindings let update it, built once per solve.
func (v *view) groupWriters() map[string][]inventory.EntityID {
	if v.writers != nil {
		return v.writers
	}
	out := map[string][]inventory.EntityID{}
	for _, kind := range []inventory.Kind{
		kube.KindClusterRoleBinding, kube.KindRoleBinding} {
		for _, binding := range v.s.Model.Entities(kind) {
			v.addBindingWriters(out, binding)
		}
	}
	v.writers = out
	return out
}

// addBindingWriters records, for each group the binding's role writes,
// the accounts the binding gives it to.
func (v *view) addBindingWriters(
	into map[string][]inventory.EntityID, binding inventory.EntityID,
) {
	e, ok := v.s.Model.Entity(binding)
	if !ok {
		return
	}
	role, ok := v.roleOf(binding, attrText(e, kube.AttrRBACRole))
	if !ok {
		return
	}
	groups := strings.Fields(attrText(role, kube.AttrRBACGroups))
	for _, entry := range strings.Fields(attrText(e,
		kube.AttrRBACAccounts)) {
		namespace, name, ok := strings.Cut(entry, "/")
		if !ok {
			continue
		}
		account := inventory.CoreID(kube.KindAccount, namespace, name)
		for _, group := range groups {
			into[group] = append(into[group], account)
		}
	}
}

// roleOf is the Role or ClusterRole a binding refers to, written
// "ClusterRole/name" or "Role/name" (a Role of the binding's namespace).
func (v *view) roleOf(
	binding inventory.EntityID, ref string,
) (inventory.Entity, bool) {
	kind, name, ok := strings.Cut(ref, "/")
	if !ok {
		return inventory.Entity{}, false
	}
	id := inventory.CoreID(kube.KindClusterRole, "", name)
	if kind == "Role" {
		id = inventory.CoreID(kube.KindRole, binding.Namespace, name)
	}
	return v.s.Model.Entity(id)
}

// failingUnitModes are the modes of a pod or workload as seen from the
// failure it explains through its pods: a node agent, the workload that
// fills a node's disk, or a controller whose own record shows nothing
// yet.
func (v *view) failingUnitModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if modes := v.agentModes(id, effect, link); len(modes) > 0 {
		return modes
	}
	if modes := v.hogModes(id, effect, link); len(modes) > 0 {
		return modes
	}
	if modes := v.noGrantModes(id, effect, link); len(modes) > 0 {
		return modes
	}
	return v.controllerModes(id, link)
}

// controllerModes are the failures of the pods of a controller workload
// that has no failing finding of its own. A Deployment reports itself
// unavailable only after minutes, while its pods already crash-loop: the
// controller is down as soon as its pods are.
func (v *view) controllerModes(
	id inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkManages || !isHandlerKind(id.Kind) ||
		len(findingModes(v.s.Findings[id])) > 0 {
		return nil
	}
	var out []modeHealth
	for _, pod := range rootcause.OwnedPods(v.s.Model, id) {
		out = append(out, findingModes(v.s.Findings[pod])...)
		out = append(out, v.containerModes(pod)...)
	}
	return out
}
