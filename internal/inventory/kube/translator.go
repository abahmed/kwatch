package kube

import (
	"time"

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

// Updated handles an informer update. Attributes and relations are always
// refreshed; a change is recorded only when the schema finds a meaningful
// difference, so resyncs and status churn never look like changes.
func (t *Translator) Updated(
	old, new any, at time.Time,
) []inventory.Observation {
	desc, ok := t.schema.Describe(new)
	if !ok {
		return nil
	}
	t.annotate(new, &desc)
	link(new, &desc)
	var previous *Description
	if oldDesc, ok := t.schema.Describe(old); ok {
		previous = &oldDesc
	}
	observations := t.describe(desc, previous, at)
	if fields := t.schema.Diff(old, new); len(fields) > 0 {
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
		Entity: desc.ID, UID: desc.UID, Attributes: desc.Attributes,
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
