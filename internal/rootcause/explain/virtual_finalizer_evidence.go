package explain

import (
	"slices"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// finalizerSubject is what the handler evidence is about: the finalizers
// holding a group of objects and the API groups their controller is
// known by.
type finalizerSubject struct {
	names  []string
	groups []string
}

// groupHandlers are the workloads known to serve the finalizers that
// hold the members, by the first tier of evidence that finds any (see
// handlersOf). Every tier searches the whole cluster: the controller
// of an API group runs where its operator was installed, which is
// rarely the namespace of the objects it holds, and a cluster-scoped
// object has no namespace at all.
func (v *view) groupHandlers(
	members []inventory.EntityID,
) []inventory.EntityID {
	if found := v.ownerHandlers(members); len(found) > 0 {
		return found
	}
	subject := v.subjectOf(members)
	tiers := []func(finalizerSubject) []inventory.EntityID{
		v.signalHandlers, v.roleHandlers, v.webhookHandlers,
	}
	for _, tier := range tiers {
		if found := tier(subject); len(found) > 0 {
			return sortedIDs(found)
		}
	}
	return nil
}

// subjectOf collects the finalizers and groups of the members.
func (v *view) subjectOf(members []inventory.EntityID) finalizerSubject {
	var out finalizerSubject
	for _, id := range members {
		out.names = append(out.names, v.finalizersOf(id)...)
		out.groups = append(out.groups, v.finalizerGroups(id,
			rootcause.OwnerChain(v.s.Model, id))...)
	}
	for _, list := range []*[]string{&out.names, &out.groups} {
		sort.Strings(*list)
		*list = slices.Compact(*list)
	}
	return out
}

// finalizerGroups are the API groups a finalizer's controller is known
// by: the domain of each finalizer and, while three labels remain, the
// domain without its leading label (the kind: "widget.widgets.example.com"
// is also "widgets.example.com"), and the groups of the object and its
// owners.
func (v *view) finalizerGroups(
	id inventory.EntityID, owners []inventory.EntityID,
) []string {
	var out []string
	for _, name := range v.finalizersOf(id) {
		domain, _, _ := strings.Cut(name, "/")
		for labels := strings.Split(domain, "."); len(labels) >= 2; {
			out = append(out, strings.Join(labels, "."))
			if len(labels) <= 3 {
				break
			}
			labels = labels[1:]
		}
	}
	for _, owner := range append([]inventory.EntityID{id}, owners...) {
		if owner.Group != "" && !builtinDomain(owner.Group) {
			out = append(out, owner.Group)
		}
	}
	sort.Strings(out)
	return out
}

// ownerHandlers (tier 1) are the workloads named, exactly, by the field
// manager that owns a finalizer entry of the members: whoever put the
// finalizer on an object is the one that removes it. One owned object
// is enough, since the finalizer is the same on all of them.
func (v *view) ownerHandlers(
	members []inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range members {
		for _, manager := range v.finalizerOwners(id) {
			if w, found := v.workloadNamed(manager,
				id.Namespace); found && !containsID(out, w) {
				out = append(out, w)
			}
		}
	}
	return sortedIDs(out)
}

// finalizerOwners are the field managers that own a finalizer holding
// the deletion of id.
func (v *view) finalizerOwners(id inventory.EntityID) []string {
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return nil
	}
	held := v.finalizersOf(id)
	var out []string
	for name, managers := range kube.FinalizerManagers(e) {
		if hasText(held, name) {
			out = append(out, managers...)
		}
	}
	sort.Strings(out)
	return out
}

// workloadNamed is the controller workload called name: one of the
// namespace wins, else exactly one elsewhere is required, since an
// ambiguous name proves nothing.
func (v *view) workloadNamed(
	name, namespace string,
) (inventory.EntityID, bool) {
	var elsewhere []inventory.EntityID
	for _, kind := range handlerKinds {
		local := inventory.CoreID(kind, namespace, name)
		if namespace != "" && v.s.Model.Exists(local) {
			return local, true
		}
		elsewhere = append(elsewhere,
			v.s.Model.CoreEntitiesNamed(kind, name)...)
	}
	if len(elsewhere) != 1 {
		return inventory.EntityID{}, false
	}
	return elsewhere[0], true
}

// signalHandlers (tier 2) are the workloads whose pods carry a label or
// annotation key in the group's domain, or whose containers quote the
// group or the finalizer in their arguments or environment.
func (v *view) signalHandlers(s finalizerSubject) []inventory.EntityID {
	var out []inventory.EntityID
	for _, kind := range handlerKinds {
		for _, w := range v.s.Model.Entities(kind) {
			if v.showsGroup(w, s) {
				out = append(out, w)
			}
		}
	}
	return out
}

// showsGroup reports whether the pod template of w names the group.
func (v *view) showsGroup(w inventory.EntityID, s finalizerSubject) bool {
	e, ok := v.s.Model.Entity(w)
	if !ok {
		return false
	}
	for key := range kube.ParseLabels(attrText(e, kube.AttrTemplateLabels)) {
		if domain, _, found := strings.Cut(key, "/"); found &&
			hasText(s.groups, domain) {
			return true
		}
	}
	for _, domain := range strings.Fields(attrText(e,
		kube.AttrTemplateAnnotationDomains)) {
		if hasText(s.groups, domain) {
			return true
		}
	}
	for _, quoted := range strings.Fields(attrText(e,
		kube.AttrTemplateQuoted)) {
		if hasText(s.groups, quoted) || hasText(s.names, quoted) {
			return true
		}
	}
	return false
}
