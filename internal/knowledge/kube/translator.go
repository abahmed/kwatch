package kube

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// FactSource is the fact source name for Kubernetes informers.
const FactSource = "kubernetes"

// Translator converts informer notifications for one schema into facts.
type Translator struct {
	schema Schema
}

// NewTranslator builds a translator for one schema.
func NewTranslator(schema Schema) *Translator {
	return &Translator{schema: schema}
}

// Added handles an informer add. Objects from the initial list are only
// observed; a genuinely new object also records a creation change.
func (t *Translator) Added(
	obj any, initialList bool, at time.Time,
) []knowledge.Fact {
	desc, ok := t.schema.Describe(obj)
	if !ok {
		return nil
	}
	facts := t.describe(desc, nil, at)
	if !initialList {
		facts = append(facts, knowledge.Fact{
			Kind: knowledge.Changed, Source: FactSource, At: at, Entity: desc.ID,
			Change: knowledge.Change{Created: true, Actor: actorOf(obj)},
		})
	}
	return facts
}

// Updated handles an informer update. Attributes and relations are always
// refreshed; a change is recorded only when the schema finds a meaningful
// difference, so resyncs and status churn never look like changes.
func (t *Translator) Updated(old, new any, at time.Time) []knowledge.Fact {
	desc, ok := t.schema.Describe(new)
	if !ok {
		return nil
	}
	var previous *Description
	if oldDesc, ok := t.schema.Describe(old); ok {
		previous = &oldDesc
	}
	facts := t.describe(desc, previous, at)
	if fields := t.schema.Diff(old, new); len(fields) > 0 {
		facts = append(facts, knowledge.Fact{
			Kind: knowledge.Changed, Source: FactSource, At: at, Entity: desc.ID,
			Change: knowledge.Change{Actor: actorOf(new), Fields: fields},
		})
	}
	return facts
}

// Deleted handles an informer delete, including tombstones.
func (t *Translator) Deleted(obj any, at time.Time) []knowledge.Fact {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	desc, ok := t.schema.Describe(obj)
	if !ok {
		return nil
	}
	facts := make([]knowledge.Fact, 0, len(desc.Children)+1)
	for _, child := range desc.Children {
		facts = append(facts, goneFact(child.ID, at))
	}
	return append(facts, goneFact(desc.ID, at))
}

func (t *Translator) describe(
	desc Description, previous *Description, at time.Time,
) []knowledge.Fact {
	facts := entityFacts(desc, t.schema.RelationTypes(), at)
	for _, child := range desc.Children {
		facts = append(facts, entityFacts(child, childRelationTypes(child), at)...)
	}
	if previous == nil {
		return facts
	}
	for _, old := range previous.Children {
		if !hasChild(desc, old.ID) {
			facts = append(facts, goneFact(old.ID, at))
		}
	}
	return facts
}

func entityFacts(
	desc Description, types []knowledge.RelationType, at time.Time,
) []knowledge.Fact {
	facts := []knowledge.Fact{{
		Kind: knowledge.Observed, Source: FactSource, At: at,
		Entity: desc.ID, UID: desc.UID, Attributes: desc.Attributes,
	}}
	for _, relation := range types {
		facts = append(facts, knowledge.Fact{
			Kind: knowledge.Related, Source: FactSource, At: at,
			Entity: desc.ID, Relation: relation,
			Targets: desc.Relations[relation],
		})
	}
	return facts
}

// childRelationTypes emits a Related fact for every type a child declares.
// Children are rebuilt with their parent, so their types are complete.
func childRelationTypes(child Description) []knowledge.RelationType {
	types := make([]knowledge.RelationType, 0, len(child.Relations))
	for relation := range child.Relations {
		types = append(types, relation)
	}
	return types
}

func hasChild(desc Description, id knowledge.EntityID) bool {
	for _, child := range desc.Children {
		if child.ID == id {
			return true
		}
	}
	return false
}

func goneFact(id knowledge.EntityID, at time.Time) knowledge.Fact {
	return knowledge.Fact{
		Kind: knowledge.Gone, Source: FactSource, At: at, Entity: id,
	}
}

func actorOf(obj any) string {
	if meta, ok := obj.(metav1.Object); ok {
		return Actor(meta)
	}
	return ""
}
