package knowledge

import "time"

// FactKind selects how a Fact updates the model.
type FactKind uint8

const (
	// Observed records that an entity exists and sets its attributes.
	Observed FactKind = iota + 1
	// Related replaces one relation type's targets for an entity.
	Related
	// Changed appends a meaningful change to the entity's history.
	Changed
	// Gone removes an entity and every relation touching it.
	Gone
)

// Fact is the only input the model accepts. Every source, from Kubernetes
// informers to a future node agent, speaks in facts.
type Fact struct {
	Kind   FactKind
	Source string
	At     time.Time
	Entity EntityID
	UID    string

	// Attributes are set by Observed facts. An attribute absent from a
	// later Observed fact from the same source is removed.
	Attributes map[string]Value

	// Relation and Targets are set by Related facts. The fact replaces the
	// entity's previous targets of that type from the same source, so a
	// source never has to diff edges itself.
	Relation RelationType
	Targets  []EntityID

	// Change is set by Changed facts.
	Change Change
}
