package knowledge

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
}

// Model is the live, indexed cluster model. It is safe for concurrent use:
// one applier writes while detectors and the reasoning engine read.
type Model struct {
	mu         sync.RWMutex
	records    map[EntityID]*record
	byKind     map[Kind]map[EntityID]struct{}
	edges      edgeIndex
	maxChanges int
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
}

type relationKeyBySource struct {
	source   string
	relation RelationType
}

// Update reports what a Fact touched. Touched always contains the fact's
// entity and, for relation changes, every added or removed target, so the
// caller can mark exactly those entities for re-evaluation.
type Update struct {
	Touched []EntityID
}

// NewModel builds an empty model.
func NewModel(options Options) *Model {
	if options.MaxChangesPerEntity <= 0 {
		options.MaxChangesPerEntity = DefaultMaxChangesPerEntity
	}
	return &Model{
		records:    make(map[EntityID]*record),
		byKind:     make(map[Kind]map[EntityID]struct{}),
		edges:      newEdgeIndex(),
		maxChanges: options.MaxChangesPerEntity,
	}
}

// Errors returned by Apply.
var (
	ErrInvalidEntity   = errors.New("fact has no entity")
	ErrInvalidRelation = errors.New("related fact has no relation type")
	ErrUnknownFactKind = errors.New("unknown fact kind")
)

// Apply folds one fact into the model.
func (m *Model) Apply(fact Fact) (Update, error) {
	if fact.Entity.IsZero() {
		return Update{}, ErrInvalidEntity
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch fact.Kind {
	case Observed:
		return m.observe(fact), nil
	case Related:
		if fact.Relation == "" {
			return Update{}, ErrInvalidRelation
		}
		return m.relate(fact), nil
	case Changed:
		return m.change(fact), nil
	case Gone:
		return m.remove(fact), nil
	default:
		return Update{}, fmt.Errorf("%w: %d", ErrUnknownFactKind, fact.Kind)
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

func (m *Model) observe(fact Fact) Update {
	rec := m.recordFor(fact.Entity)
	if !rec.present {
		rec.present = true
		rec.goneAt = time.Time{}
		rec.entity.FirstSeen = fact.At
		m.indexKind(fact.Entity)
	}
	if fact.UID != "" {
		rec.entity.UID = fact.UID
	}
	for name, source := range rec.attrSource {
		if _, still := fact.Attributes[name]; source == fact.Source && !still {
			delete(rec.entity.Attributes, name)
			delete(rec.attrSource, name)
		}
	}
	for name, value := range fact.Attributes {
		current, ok := rec.entity.Attributes[name]
		if ok && current.Value.Equal(value) {
			current.Updated = fact.At
			rec.entity.Attributes[name] = current
		} else {
			rec.entity.Attributes[name] = Attribute{
				Value: value, Since: fact.At, Updated: fact.At,
			}
		}
		rec.attrSource[name] = fact.Source
	}
	return Update{Touched: []EntityID{fact.Entity}}
}

func (m *Model) relate(fact Fact) Update {
	rec := m.recordFor(fact.Entity)
	key := relationKeyBySource{source: fact.Source, relation: fact.Relation}
	previous := rec.relations[key]
	next := uniqueIDs(fact.Targets)
	touched := []EntityID{fact.Entity}
	for _, target := range previous {
		if !containsID(next, target) {
			m.edges.remove(fact.Entity, fact.Relation, target)
			touched = append(touched, target)
		}
	}
	for _, target := range next {
		if !containsID(previous, target) {
			m.edges.add(fact.Entity, fact.Relation, target)
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

func (m *Model) change(fact Fact) Update {
	rec := m.recordFor(fact.Entity)
	change := fact.Change
	change.Entity = fact.Entity
	if change.At.IsZero() {
		change.At = fact.At
	}
	rec.changes = append(rec.changes, change)
	if overflow := len(rec.changes) - m.maxChanges; overflow > 0 {
		rec.changes = append(rec.changes[:0:0], rec.changes[overflow:]...)
	}
	return Update{Touched: []EntityID{fact.Entity}}
}

// remove drops the entity, its attributes and the relations it declared.
// Relations other entities declare towards it are kept: a pod that still
// references a deleted Secret is exactly the evidence rules need.
func (m *Model) remove(fact Fact) Update {
	rec := m.records[fact.Entity]
	if rec == nil || !rec.present {
		return Update{}
	}
	touched := []EntityID{fact.Entity}
	for key, targets := range rec.relations {
		for _, target := range targets {
			m.edges.remove(fact.Entity, key.relation, target)
			touched = append(touched, target)
		}
	}
	rec.relations = make(map[relationKeyBySource][]EntityID)
	rec.entity.Attributes = make(map[string]Attribute)
	rec.attrSource = make(map[string]string)
	rec.present = false
	rec.goneAt = fact.At
	m.unindexKind(fact.Entity)
	m.change(Fact{
		Entity: fact.Entity, At: fact.At,
		Change: Change{Deleted: true, Actor: fact.Change.Actor},
	})
	return Update{Touched: touched}
}

func (m *Model) indexKind(id EntityID) {
	ids := m.byKind[id.Kind]
	if ids == nil {
		ids = make(map[EntityID]struct{})
		m.byKind[id.Kind] = ids
	}
	ids[id] = struct{}{}
}

func (m *Model) unindexKind(id EntityID) {
	delete(m.byKind[id.Kind], id)
	if len(m.byKind[id.Kind]) == 0 {
		delete(m.byKind, id.Kind)
	}
}

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
