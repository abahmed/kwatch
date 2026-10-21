package inventory

import "time"

// Entity returns a detached snapshot of a present entity.
func (m *Model) Entity(id EntityID) (Entity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[id]
	if rec == nil || !rec.present {
		return Entity{}, false
	}
	return rec.snapshot(), true
}

// Attribute returns one attribute of a present entity without copying the
// entity: one read lock and one binary search, no allocation. Hot paths
// that need a single attribute use it instead of Entity.
func (m *Model) Attribute(id EntityID, name string) (Attribute, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[id]
	if rec == nil || !rec.present {
		return Attribute{}, false
	}
	return rec.attribute(name)
}

// GoneAttribute returns an attribute a source handed over when it deleted
// the entity, such as the accounts a deleted binding bound. A present
// entity has none, and neither does one deleted without it (for example
// while kwatch was down).
func (m *Model) GoneAttribute(id EntityID, name string) (Value, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[id]
	if rec == nil || rec.present {
		return Value{}, false
	}
	value, ok := rec.goneAttrs[name]
	return value, ok
}

// Exists reports whether the entity is currently present.
func (m *Model) Exists(id EntityID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[id]
	return rec != nil && rec.present
}

// Related returns the entities joined to id by one relation type, in
// deterministic order. Targets may be absent entities: a reference to a
// missing object is still a relation.
func (m *Model) Related(
	id EntityID, relation RelationType, dir Direction,
) []EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.edges.neighbors(id, relation, dir)
}

// Relations returns every relation of id in one direction.
func (m *Model) Relations(id EntityID, dir Direction) []Relation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.edges.relations(id, dir)
}

// Entities returns the present entities of one kind, sorted by their
// string form, so callers never need to sort the result again.
func (m *Model) Entities(kind Kind) []EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []EntityID
	for _, ids := range m.byKind[kind] {
		for id := range ids {
			out = append(out, id)
		}
	}
	sortIDs(out)
	return out
}

// Changes returns the entity's changes at or after since, oldest first.
// Deleted entities keep their history until pruned.
func (m *Model) Changes(id EntityID, since time.Time) []Change {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[id]
	if rec == nil {
		return nil
	}
	var out []Change
	for _, change := range rec.changes {
		if !change.At.Before(since) {
			out = append(out, cloneChange(change))
		}
	}
	return out
}

// Stats summarises the model size for health and metrics.
type Stats struct {
	Entities   int
	Tombstones int
	Relations  int
	Changes    int
	// Notes counts event notes, NoteParts the per-origin counters behind
	// them, and Health the failing/recovered marks, over all records.
	Notes     int
	NoteParts int
	Health    int
	// Recent and Churn are the cluster-wide change rings.
	Recent int
	Churn  int
	// Baselines is how many workloads have a rolling baseline.
	Baselines int
}

// Stats returns the current model size.
func (m *Model) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := Stats{
		Relations: m.edges.n, Recent: len(m.history.recent),
		Churn: len(m.history.churn), Baselines: m.history.baselines.Len(),
	}
	for _, rec := range m.records {
		if rec.present {
			stats.Entities++
		} else {
			stats.Tombstones++
		}
		stats.Changes += len(rec.changes)
		stats.Notes += len(rec.notes)
		stats.NoteParts += len(rec.noteParts)
		stats.Health += len(rec.health)
	}
	return stats
}

// Prune drops changes older than before, and tombstones whose history is
// then empty. The owner calls it periodically; the persistent store keeps
// the long history.
func (m *Model) Prune(before time.Time) int {
	return m.PruneKeeping(before, before)
}

// PruneKeeping is Prune with a longer memory for changes and health
// marks: notes older than before go, changes and marks older than
// keepFrom go. A restart reloads a day of changes, so the owner keeps
// that day while notes stay short-lived.
func (m *Model) PruneKeeping(before, keepFrom time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history.prune(keepFrom)
	removed := 0
	for id, rec := range m.records {
		rec.notes = pruneNotes(rec.notes, before)
		rec.dropOrphanParts()
		kept := rec.changes[:0]
		for _, change := range rec.changes {
			if !change.At.Before(keepFrom) {
				kept = append(kept, change)
			}
		}
		removed += len(rec.changes) - len(kept)
		rec.changes = kept
		rec.health = pruneHealth(rec.health, keepFrom)
		// Records that hold nothing any more are dropped: tombstones
		// whose history, notes and health marks expired.
		if !rec.present && len(rec.changes) == 0 &&
			len(rec.relations) == 0 && len(rec.notes) == 0 &&
			len(rec.health) == 0 && rec.goneAt.Before(before) {
			delete(m.records, id)
		}
	}
	return removed
}

func cloneChange(change Change) Change {
	change.Fields = append([]FieldChange(nil), change.Fields...)
	return change
}

// Notes returns the entity's notes at or after since, oldest first.
func (m *Model) Notes(id EntityID, since time.Time) []Note {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[id]
	if rec == nil {
		return nil
	}
	var out []Note
	for _, note := range rec.notes {
		if !note.At.Before(since) && !m.aboutOlderObject(rec, note) {
			out = append(out, note)
		}
	}
	return out
}

// NotedKinds lists the kinds of the entities that have a Warning note
// since the given time, so a watch budget can favour them.
func (m *Model) NotedKinds(since time.Time) map[Kind]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[Kind]bool{}
	for id, rec := range m.records {
		if out[id.Kind] {
			continue
		}
		for _, note := range rec.notes {
			if note.Warning && !note.At.Before(since) &&
				!m.aboutOlderObject(rec, note) {
				out[id.Kind] = true
				break
			}
		}
	}
	return out
}

func pruneNotes(notes []Note, before time.Time) []Note {
	kept := notes[:0]
	for _, note := range notes {
		if !note.At.Before(before) {
			kept = append(kept, note)
		}
	}
	return kept
}
