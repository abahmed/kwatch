package inventory

import "time"

// PinnedRetention is how long a pinned change stays in memory: as long
// as the state file keeps changes on disk. Pinned changes are the older
// history a restart reloads for the entities of open incidents, so an
// incident open for days keeps the change it blamed and can tell when
// that change is reverted.
const PinnedRetention = 7 * 24 * time.Hour

// DefaultMaxPinnedChanges bounds the ring of pinned changes.
const DefaultMaxPinnedChanges = 4096

// pinChange adds a restored change that outlives ChangeRetention: to its
// entity's history, which is then pruned at PinnedRetention, and to the
// pinned ring change sets are built from. The caller holds the lock.
func (m *Model) pinChange(change Change) {
	rec := m.recordFor(change.Entity)
	rec.pinned = true
	rec.changes = boundChanges(append(rec.changes, change), m.maxChanges)
	h := &m.history
	h.seq++
	entry := changeEntry{seq: h.seq, change: cloneChange(change)}
	h.pinned = h.appendBounded(h.pinned, entry, DefaultMaxPinnedChanges)
}

// pinnedFrom is how far back pinned changes are kept when the others
// are kept from keepFrom.
func pinnedFrom(keepFrom time.Time) time.Time {
	return keepFrom.Add(ChangeRetention - PinnedRetention)
}

// pruneChanges drops rec's changes older than keepFrom, or than
// PinnedRetention while it holds pinned changes, and returns how many it
// dropped. Once nothing older than keepFrom is left, the entity is no
// longer pinned.
func (rec *record) pruneChanges(keepFrom time.Time) int {
	from := keepFrom
	if rec.pinned {
		from = pinnedFrom(keepFrom)
	}
	kept := rec.changes[:0]
	for _, change := range rec.changes {
		if !change.At.Before(from) {
			kept = append(kept, change)
		}
	}
	removed := len(rec.changes) - len(kept)
	clear(rec.changes[len(kept):])
	rec.changes = kept
	if len(kept) == 0 || !kept[0].At.Before(keepFrom) {
		rec.pinned = false
	}
	return removed
}

// Unpin ends the pinning of ids, as when the incident their changes
// were pinned for resolves: their changes older than keepFrom, the
// start of ChangeRetention, leave their histories and the pinned ring.
// It returns how many changes left the ring.
func (m *Model) Unpin(ids []EntityID, keepFrom time.Time) int {
	if len(ids) == 0 {
		return 0
	}
	drop := make(map[EntityID]bool, len(ids))
	for _, id := range ids {
		drop[id] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range drop {
		if rec := m.records[id]; rec != nil && rec.pinned {
			rec.pinned = false
			rec.pruneChanges(keepFrom)
		}
	}
	h := &m.history
	kept := h.pinned[:0]
	for _, entry := range h.pinned {
		if !drop[entry.change.Entity] ||
			!entry.change.At.Before(keepFrom) {
			kept = append(kept, entry)
		}
	}
	removed := len(h.pinned) - len(kept)
	h.evictions += uint64(removed)
	clear(h.pinned[len(kept):])
	h.pinned = kept
	return removed
}
