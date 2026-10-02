package inventory

import "time"

// Reader is the read side of the model. Detectors and the reasoning engine
// depend on it rather than on *Model, so they can be tested with fixtures.
type Reader interface {
	Entity(id EntityID) (Entity, bool)
	Exists(id EntityID) bool
	Related(id EntityID, relation RelationType, dir Direction) []EntityID
	Relations(id EntityID, dir Direction) []Relation
	Entities(kind Kind) []EntityID
	EntitiesIn(kind Kind, namespace string) []EntityID
	CoreEntitiesNamed(kind Kind, name string) []EntityID
	AttributeIn(kind Kind, namespace, attribute string) map[EntityID]Value
	Changes(id EntityID, since time.Time) []Change
	Notes(id EntityID, since time.Time) []Note
}

var _ Reader = (*Model)(nil)

// HistoryReader adds the model's change history: change sets, their
// outcomes, health marks and baselines. Root-cause rules and the
// incident manager use it for "what changed, and what did it do". All
// methods are pure reads except through Baselines.
type HistoryReader interface {
	Reader
	ChangeSets(id EntityID, since time.Time) []ChangeSet
	RecentChangeSets(since time.Time) []ChangeSet
	ChangeOutcomes(id EntityID, since, now time.Time) []ChangeOutcome
	OutcomeOf(set ChangeSet, now time.Time) ChangeOutcome
	HealthMarks(id EntityID, since time.Time) []HealthMark
	Baselines() *Baselines
}

var _ HistoryReader = (*Model)(nil)
