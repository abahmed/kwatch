package kube

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ObservationSource is the observation source name for Kubernetes informers.
const ObservationSource = "kubernetes"

// Translator converts informer notifications for one schema into observations.
type Translator struct {
	schema      Schema
	maintenance MaintenanceAnnotations
}

// NewTranslator builds a translator for one schema.
func NewTranslator(schema Schema) *Translator {
	return &Translator{schema: schema}
}

// Added handles an informer add. Objects from the initial list are only
// observed; a genuinely new object also records a creation change.
func (t *Translator) Added(
	obj any, initialList bool, at time.Time,
) []inventory.Observation {
	desc, ok := t.schema.Describe(obj)
	if !ok {
		return nil
	}
	t.annotate(obj, &desc)
	link(obj, &desc)
	observations := t.describe(desc, nil, at)
	if !initialList {
		observations = append(observations, inventory.Observation{
			Kind: inventory.Changed, Source: ObservationSource, At: at, Entity: desc.ID,
			Change: attributedChange(obj, true, at, nil),
		})
	}
	return observations
}

// Updated handles an informer update. Attributes and relations are
// refreshed; a change is recorded only when the schema finds a meaningful
// difference, so status churn never looks like a change. A periodic
// resync repeats an object at the same resource version: it is skipped,
// because every schema describes an object from its content alone, so
// the model already holds exactly what the resync would say. A new UID
// under the same name is a re-created object (the watch missed the delete)
// and is told as a delete followed by a create.
func (t *Translator) Updated(
	old, new any, at time.Time,
) []inventory.Observation {
	desc, ok := t.schema.Describe(new)
	if !ok {
		return nil
	}
	if sameVersion(old, new) {
		return nil
	}
	if oldDesc, ok := t.schema.Describe(old); ok &&
		oldDesc.UID != "" && desc.UID != "" && oldDesc.UID != desc.UID {
		return append(t.Deleted(old, at), t.Added(new, false, at)...)
	}
	t.annotate(new, &desc)
	link(new, &desc)
	var previous *Description
	if oldDesc, ok := t.schema.Describe(old); ok {
		previous = &oldDesc
	}
	observations := t.describe(desc, previous, at)
	if fields := boundFields(t.schema.Diff(old, new)); len(fields) > 0 {
		observations = append(observations, inventory.Observation{
			Kind: inventory.Changed, Source: ObservationSource, At: at, Entity: desc.ID,
			Change: attributedChange(new, false, at, fields),
		})
	}
	return observations
}

// Deleted handles an informer delete. Tombstones are unwrapped by
// kubeclient.SafeEventHandler before they reach here.
func (t *Translator) Deleted(obj any, at time.Time) []inventory.Observation {
	desc, ok := t.schema.Describe(obj)
	if !ok {
		return nil
	}
	observations := make([]inventory.Observation, 0, len(desc.Children)+1)
	for _, child := range desc.Children {
		observations = append(observations, goneObservation(child.ID, at))
	}
	return append(observations, goneObservation(desc.ID, at))
}

func (t *Translator) describe(
	desc Description, previous *Description, at time.Time,
) []inventory.Observation {
	observations := entityObservations(desc, t.schema.RelationTypes(), at)
	for _, child := range desc.Children {
		observations = append(observations, entityObservations(
			child, childRelationTypes(child), at)...)
	}
	if previous == nil {
		return observations
	}
	for _, old := range previous.Children {
		if !hasChild(desc, old.ID) {
			observations = append(observations, goneObservation(old.ID, at))
		}
	}
	return observations
}

func entityObservations(
	desc Description, types []inventory.RelationType, at time.Time,
) []inventory.Observation {
	observations := []inventory.Observation{{
		Kind: inventory.Observed, Source: ObservationSource, At: at,
		Entity: desc.ID, UID: desc.UID, AltUID: desc.AltUID,
		Attributes: desc.Attributes,
	}}
	for _, relation := range types {
		observations = append(observations, inventory.Observation{
			Kind: inventory.Related, Source: ObservationSource, At: at,
			Entity: desc.ID, Relation: relation,
			Targets: desc.Relations[relation],
		})
	}
	return observations
}

// childRelationTypes emits a Related observation for every type a child
// declares. Children are rebuilt with their parent, so their types are
// complete.
func childRelationTypes(child Description) []inventory.RelationType {
	types := make([]inventory.RelationType, 0, len(child.Relations))
	for relation := range child.Relations {
		types = append(types, relation)
	}
	return types
}

func hasChild(desc Description, id inventory.EntityID) bool {
	for _, child := range desc.Children {
		if child.ID == id {
			return true
		}
	}
	return false
}

func goneObservation(
	id inventory.EntityID, at time.Time,
) inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Gone, Source: ObservationSource, At: at, Entity: id,
	}
}

// sameVersion reports the same object at the same resource version, which
// is what a periodic resync delivers. Objects without a version are never
// the same.
func sameVersion(old, new any) bool {
	before, err1 := meta.Accessor(old)
	after, err2 := meta.Accessor(new)
	if err1 != nil || err2 != nil {
		return false
	}
	return before.GetResourceVersion() != "" &&
		before.GetResourceVersion() == after.GetResourceVersion() &&
		before.GetUID() == after.GetUID()
}
