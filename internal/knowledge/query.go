package knowledge

import "time"

// Entity returns a detached snapshot of a present entity.
func (m *Model) Entity(id EntityID) (Entity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[id]
	if rec == nil || !rec.present {
		return Entity{}, false
	}
	return rec.entity.clone(), true
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

// Entities returns the present entities of one kind in deterministic
// order.
func (m *Model) Entities(kind Kind) []EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]EntityID, 0, len(m.byKind[kind]))
	for id := range m.byKind[kind] {
		out = append(out, id)
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
}

// Stats returns the current model size.
func (m *Model) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := Stats{Relations: m.edges.n}
	for _, rec := range m.records {
		if rec.present {
			stats.Entities++
		} else {
			stats.Tombstones++
		}
		stats.Changes += len(rec.changes)
	}
	return stats
}

// Prune drops changes older than before, and tombstones whose history is
// then empty. The owner calls it periodically; the persistent store keeps
// the long history.
func (m *Model) Prune(before time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, rec := range m.records {
		rec.notes = pruneNotes(rec.notes, before)
		kept := rec.changes[:0]
		for _, change := range rec.changes {
			if !change.At.Before(before) {
				kept = append(kept, change)
			}
		}
		removed += len(rec.changes) - len(kept)
		rec.changes = kept
		// Records that hold nothing any more are dropped: tombstones
		// whose history expired, and notes about never-observed objects.
		if !rec.present && len(rec.changes) == 0 &&
			len(rec.relations) == 0 && len(rec.notes) == 0 &&
			rec.goneAt.Before(before) {
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
		if !note.At.Before(since) {
			out = append(out, note)
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
