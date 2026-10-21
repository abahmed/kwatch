package inventory

import (
	"errors"
	"fmt"
	"maps"
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
	// entity holds the identity; its Attributes map stays nil, the
	// attributes live in attrs (see attr_store.go).
	entity    Entity
	present   bool
	goneAt    time.Time
	attrs     []attrEntry
	relations []relationEntry
	changes   []Change
	notes     []Note
	noteParts map[noteKey][]originCount
	health    []HealthMark
	// altUID is a second identifier events may use for the object, such
	// as the config hash of a static pod's mirror.
	altUID string
	// goneAttrs are the attributes a source handed over with the
	// deletion: the few facts about the object that stay readable on its
	// tombstone, such as who a deleted binding bound. See GoneAttribute.
	goneAttrs map[string]Value
}

type relationKeyBySource struct {
	source   string
	relation RelationType
}

// relationEntry is what one source declared for one relation type. A
// record has only a few, so a slice beats a map.
type relationEntry struct {
	key     relationKeyBySource
	targets []EntityID
}

func (r *record) relationTargets(key relationKeyBySource) []EntityID {
	for _, entry := range r.relations {
		if entry.key == key {
			return entry.targets
		}
	}
	return nil
}

// setRelationTargets stores the targets, or forgets the key when there
// are none.
func (r *record) setRelationTargets(
	key relationKeyBySource, targets []EntityID,
) {
	for i, entry := range r.relations {
		if entry.key != key {
			continue
		}
		if len(targets) == 0 {
			r.relations = append(r.relations[:i], r.relations[i+1:]...)
		} else {
			r.relations[i].targets = targets
		}
		return
	}
	if len(targets) > 0 {
		r.relations = append(r.relations,
			relationEntry{key: key, targets: targets})
	}
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
		update := m.observe(observation)
		update.Touched = append(update.Touched,
			m.backedReferrers(observation.Entity)...)
		return update, nil
	case Related:
		if observation.Relation == "" {
			return Update{}, ErrInvalidRelation
		}
		update := m.relate(observation)
		update.Touched = append(update.Touched,
			m.backedReferrers(observation.Entity)...)
		return update, nil
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
			entity: Entity{ID: id},
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
		rec.goneAttrs = nil
		rec.entity.FirstSeen = observation.At
		m.indexKind(observation.Entity)
	}
	rec.adoptUID(observation.UID, observation.AltUID)
	rec.dropAttributesFrom(observation.Source, observation.Attributes)
	if rec.attrs == nil {
		rec.attrs = make([]attrEntry, 0, len(observation.Attributes))
	}
	for name, value := range observation.Attributes {
		rec.setAttribute(name, observation.Source, value, observation.At)
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
	previous := rec.relationTargets(key)
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
	rec.setRelationTargets(key, next)
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

// remove drops the entity, its attributes and the relations it declared.
// Relations other entities declare towards it are kept: a pod that still
// references a deleted Secret is exactly the evidence rules need.
func (m *Model) remove(observation Observation) Update {
	rec := m.records[observation.Entity]
	if rec == nil || !rec.present {
		return Update{}
	}
	touched := []EntityID{observation.Entity}
	for _, entry := range rec.relations {
		for _, target := range entry.targets {
			m.edges.remove(observation.Entity, entry.key.relation, target)
			touched = append(touched, target)
		}
	}
	rec.relations = nil
	rec.attrs = nil
	rec.goneAttrs = maps.Clone(observation.Attributes)
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
