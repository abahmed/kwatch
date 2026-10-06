package kube

import (
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Link is one resolved generic link from an entity to a present target.
type Link struct {
	Type inventory.RelationType
	To   inventory.EntityID
	// Inferred marks a medium-confidence link derived from a naming
	// convention rather than a reference field.
	Inferred bool
}

// managerKinds are the controller workloads a field manager name may
// resolve to, in preference order.
var managerKinds = []inventory.Kind{
	KindDeployment, KindStatefulSet, KindDaemonSet,
}

// Links resolves an entity's generic links against the inventory at read
// time. Edges and attributes are stored as candidates; a link is returned
// only when its target is present now, so a missing target never shows up
// and a target created later appears without re-describing the source.
// Selects is resolved from the selector attribute to pods of the same
// namespace, and managed-by from the spec manager name.
func Links(r inventory.Reader, id inventory.EntityID) []Link {
	var out []Link
	for _, relation := range []inventory.RelationType{
		inventory.References, inventory.Serves, inventory.Calls,
	} {
		for _, target := range r.Related(id, relation, inventory.Outgoing) {
			if r.Exists(target) {
				out = append(out, Link{Type: relation, To: target})
			}
		}
	}
	entity, ok := r.Entity(id)
	if !ok {
		return out
	}
	for _, pod := range selectedPods(r, entity) {
		out = append(out, Link{Type: inventory.Selects, To: pod})
	}
	if manager, ok := managerOf(r, entity); ok {
		out = append(out, Link{
			Type: inventory.ManagedBy, To: manager, Inferred: true,
		})
	}
	return out
}

// selectedPods returns the pods in the entity's namespace matching its
// selector. "<none>" is an empty LabelSelector and selects every pod; an
// absent or unparsable selector selects nothing. Cluster-scoped
// entities select nothing: a selector alone does not say which
// namespaces it spans.
func selectedPods(
	r inventory.Reader, e inventory.Entity,
) []inventory.EntityID {
	text := entityText(e, AttrSelector)
	if text == "" || e.ID.Namespace == "" {
		return nil
	}
	selector := labels.Everything()
	if text != "<none>" {
		parsed, err := labels.Parse(text)
		if err != nil {
			return nil
		}
		selector = parsed
	}
	// Only this namespace's pod labels are read, from the model's
	// kind+namespace index, instead of every pod in the cluster.
	podLabels := r.AttributeIn(KindPod, e.ID.Namespace, AttrLabels)
	var out []inventory.EntityID
	for _, id := range r.EntitiesIn(KindPod, e.ID.Namespace) {
		if id.Group != "" {
			continue
		}
		set, err := labels.ConvertSelectorToLabelsMap(
			podLabels[id].AsText())
		if err == nil && selector.Matches(set) {
			out = append(out, id)
		}
	}
	return out
}

// managerOf resolves the spec manager name to a controller workload. A
// workload in the entity's namespace wins; otherwise exactly one match
// elsewhere is required, since an ambiguous name proves nothing.
func managerOf(
	r inventory.Reader, e inventory.Entity,
) (inventory.EntityID, bool) {
	name := entityText(e, AttrManagedBy)
	if name == "" {
		return inventory.EntityID{}, false
	}
	var elsewhere []inventory.EntityID
	for _, kind := range managerKinds {
		local := inventory.CoreID(kind, e.ID.Namespace, name)
		if local != e.ID && e.ID.Namespace != "" && r.Exists(local) {
			return local, true
		}
		for _, id := range r.CoreEntitiesNamed(kind, name) {
			if id != e.ID {
				elsewhere = append(elsewhere, id)
			}
		}
	}
	if len(elsewhere) != 1 {
		return inventory.EntityID{}, false
	}
	return elsewhere[0], true
}

func entityText(e inventory.Entity, name string) string {
	attribute, ok := e.Attribute(name)
	if !ok {
		return ""
	}
	return attribute.Value.AsText()
}
