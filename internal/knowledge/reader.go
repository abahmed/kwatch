package knowledge

import "time"

// Reader is the read side of the model. Detectors and the reasoning engine
// depend on it rather than on *Model, so they can be tested with fixtures.
type Reader interface {
	Entity(id EntityID) (Entity, bool)
	Exists(id EntityID) bool
	Related(id EntityID, relation RelationType, dir Direction) []EntityID
	Relations(id EntityID, dir Direction) []Relation
	Entities(kind Kind) []EntityID
	Changes(id EntityID, since time.Time) []Change
}

var _ Reader = (*Model)(nil)
