package inventory

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultMaxChangesPerEntity bounds the in-memory change history of one
// entity. Older changes live in the persistent store.
const DefaultMaxChangesPerEntity = 64

// Options configures a Model.
type Options struct {
	MaxChangesPerEntity int
	// EnrichmentSources name sources that are not authoritative for an
	// entity's existence, such as kubelet usage stats. Their Observed
	// observations update attributes of a present entity but never create an
	// entity or resurrect one that is gone.
	EnrichmentSources []string
}

// Model is the live, indexed cluster model. It is safe for concurrent use:
// one applier writes while detectors and the reasoning engine read.
type Model struct {
	mu         sync.RWMutex
	records    map[EntityID]*record
	byKind     kindIndex
	edges      edgeIndex
	maxChanges int
	enrichment map[string]bool
	// history holds cluster-wide recent changes, entity health marks and
	// workload baselines for change sets, outcomes and recurrence.
	history history
}

// record is the mutable state behind one entity. A record outlives its
// entity as a tombstone while it still carries change history, so "this
// Secret was deleted two minutes ago" remains answerable.
type record struct {
	entity     Entity
	present    bool
	goneAt     time.Time
	attrSource map[string]string
	relations  map[relationKeyBySource][]EntityID
	changes    []Change
	notes      []Note
	health     []HealthMark
}

type relationKeyBySource struct {
	source   string
	relation RelationType
}

// Update reports what an Observation touched. Touched always contains the
// observation's entity and, for relation changes, every added or removed
// target, so the caller can mark exactly those entities for re-evaluation.
//
// Appeared is true when the observation made an absent entity present:
// whatever referenced it while it was missing must be judged again.
type Update struct {
	Touched  []EntityID
	Appeared bool
}

// NewModel builds an empty model.
func NewModel(options Options) *Model {
	if options.MaxChangesPerEntity <= 0 {
		options.MaxChangesPerEntity = DefaultMaxChangesPerEntity
	}
	enrichment := make(map[string]bool, len(options.EnrichmentSources))
	for _, source := range options.EnrichmentSources {
		enrichment[source] = true
	}
	return &Model{
		records:    make(map[EntityID]*record),
		byKind:     make(kindIndex),
		edges:      newEdgeIndex(),
		maxChanges: options.MaxChangesPerEntity,
		enrichment: enrichment,
		history:    newHistory(),
	}
}

// Errors returned by Apply.
var (
	ErrInvalidEntity   = errors.New("observation has no entity")
	ErrInvalidRelation = errors.New(
		"related observation has no relation type")
	ErrUnknownObservationKind = errors.New("unknown observation kind")
)

// Apply folds one observation into the model.
func (m *Model) Apply(observation Observation) (Update, error) {
	if observation.Entity.IsZero() {
		return Update{}, ErrInvalidEntity
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch observation.Kind {
	case Observed:
		return m.observe(observation), nil
	case Related:
		if observation.Relation == "" {
			return Update{}, ErrInvalidRelation
		}
		return m.relate(observation), nil
	case Changed:
		return m.change(observation), nil
	case Gone:
		return m.remove(observation), nil
	case Noted:
		return m.note(observation), nil
	default:
		return Update{}, fmt.Errorf("%w: %d",
			ErrUnknownObservationKind, observation.Kind)
	}
}

func (m *Model) recordFor(id EntityID) *record {
	rec := m.records[id]
	if rec == nil {
		rec = &record{
			entity: Entity{
				ID: id, Attributes: make(map[string]Attribute),
			},
			attrSource: make(map[string]string),
			relations:  make(map[relationKeyBySource][]EntityID),
		}
		m.records[id] = rec
	}
	return rec
}

func (m *Model) observe(observation Observation) Update {
	if m.enrichment[observation.Source] {
		if rec := m.records[observation.Entity]; rec == nil || !rec.present {
			return Update{}
		}
	}
	rec := m.recordFor(observation.Entity)
	appeared := !rec.present
	if appeared {
		rec.present = true
		rec.goneAt = time.Time{}
		rec.entity.FirstSeen = observation.At
		m.indexKind(observation.Entity)
	}
	if observation.UID != "" {
		rec.entity.UID = observation.UID
	}
	for name, source := range rec.attrSource {
		_, still := observation.Attributes[name]
		if source == observation.Source && !still {
			delete(rec.entity.Attributes, name)
			delete(rec.attrSource, name)
		}
	}
	for name, value := range observation.Attributes {
		current, ok := rec.entity.Attributes[name]
		if ok && current.Value.Equal(value) {
			current.Updated = observation.At
			rec.entity.Attributes[name] = current
		} else {
			rec.entity.Attributes[name] = Attribute{
				Value: value, Since: observation.At, Updated: observation.At,
			}
		}
		rec.attrSource[name] = observation.Source
	}
	return Update{
		Touched: []EntityID{observation.Entity}, Appeared: appeared,
	}
}

func (m *Model) relate(observation Observation) Update {
	rec := m.recordFor(observation.Entity)
	key := relationKeyBySource{
		source: observation.Source, relation: observation.Relation,
	}
	previous := rec.relations[key]
	next := uniqueIDs(observation.Targets)
	touched := []EntityID{observation.Entity}
	for _, target := range previous {
		if !containsID(next, target) {
			m.edges.remove(observation.Entity, observation.Relation, target)
			touched = append(touched, target)
		}
	}
	for _, target := range next {
		if !containsID(previous, target) {
			m.edges.add(observation.Entity, observation.Relation, target)
			touched = append(touched, target)
		}
	}
	if len(next) == 0 {
		delete(rec.relations, key)
	} else {
		rec.relations[key] = next
	}
	return Update{Touched: touched}
}

func (m *Model) change(observation Observation) Update {
	rec := m.recordFor(observation.Entity)
	change := observation.Change
	change.Entity = observation.Entity
	if change.At.IsZero() {
		change.At = observation.At
	}
	rec.changes = append(rec.changes, change)
	m.history.addChange(change)
	if overflow := len(rec.changes) - m.maxChanges; overflow > 0 {
		rec.changes = append(rec.changes[:0:0], rec.changes[overflow:]...)
	}
	return Update{Touched: []EntityID{observation.Entity}}
}

func (m *Model) note(observation Observation) Update {
	rec := m.recordFor(observation.Entity)
	note := observation.Note
	if note.At.IsZero() {
		note.At = observation.At
	}
	if note.FirstSeen.IsZero() || note.FirstSeen.After(note.At) {
		note.FirstSeen = note.At
	}
	for i, existing := range rec.notes {
		if existing.Source == note.Source && existing.Reason == note.Reason {
			note.FirstSeen = earliest(existing.FirstSeen, note.FirstSeen)
			rec.notes = append(rec.notes[:i], rec.notes[i+1:]...)
			break
		}
	}
	rec.notes = append(rec.notes, note)
	if over := len(rec.notes) - DefaultMaxNotesPerEntity; over > 0 {
		rec.notes = append(rec.notes[:0:0], rec.notes[over:]...)
	}
	return Update{Touched: []EntityID{observation.Entity}}
}

// remove drops the entity, its attributes and the relations it declared.
// Relations other entities declare towards it are kept: a pod that still
// references a deleted Secret is exactly the evidence rules need.
func (m *Model) remove(observation Observation) Update {
	rec := m.records[observation.Entity]
	if rec == nil || !rec.present {
		return Update{}
	}
	touched := []EntityID{observation.Entity}
	for key, targets := range rec.relations {
		for _, target := range targets {
			m.edges.remove(observation.Entity, key.relation, target)
			touched = append(touched, target)
		}
	}
	rec.relations = make(map[relationKeyBySource][]EntityID)
	rec.entity.Attributes = make(map[string]Attribute)
	rec.attrSource = make(map[string]string)
	rec.present = false
	rec.goneAt = observation.At
	m.unindexKind(observation.Entity)
	m.change(Observation{
		Entity: observation.Entity, At: observation.At,
		Change: Change{Deleted: true, Actor: observation.Change.Actor},
	})
	return Update{Touched: touched}
}

func (m *Model) indexKind(id EntityID) { m.byKind.add(id) }

func (m *Model) unindexKind(id EntityID) { m.byKind.remove(id) }

func uniqueIDs(ids []EntityID) []EntityID {
	out := make([]EntityID, 0, len(ids))
	for _, id := range ids {
		if !id.IsZero() && !containsID(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func containsID(ids []EntityID, id EntityID) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
