package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A detector that needs a kind kwatch cannot verify now (its access was
// lost, or its watch keeps failing) holds what it raised: neither
// raise nor clear. The incident then stays open on findings nobody can
// check, which is right, but it must say so once the hold lasts.

// UnverifiedAfter is how long findings must be held before the
// incident says kwatch cannot verify it.
const UnverifiedAfter = 10 * time.Minute

// NoteHeld records whether the latest evaluation of id held its
// findings over a kind it could not verify. The first hold is
// remembered until an evaluation decides again.
func (m *Manager) NoteHeld(id inventory.EntityID, held bool, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !held {
		delete(m.heldSince, id)
		return
	}
	if _, ok := m.heldSince[id]; ok {
		return
	}
	if m.heldSince == nil {
		m.heldSince = map[inventory.EntityID]time.Time{}
	}
	m.heldSince[id] = now
}

// Unverified says in one line that kwatch cannot verify incident id
// right now, when a finding of it has been held for UnverifiedAfter;
// "" otherwise.
func (m *Manager) Unverified(id string, now time.Time) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil || p.State == Resolved || len(m.heldSince) == 0 {
		return ""
	}
	var kinds []inventory.Kind
	for _, f := range p.Members {
		since, ok := m.heldSince[f.Entity]
		if ok && now.Sub(since) >= UnverifiedAfter {
			kinds = append(kinds, f.Entity.Kind)
		}
	}
	if len(kinds) == 0 {
		return ""
	}
	kind := slices.Min(kinds)
	if m.cfg.Verifiable != nil && !m.cfg.Verifiable(kind) {
		return "kwatch can't verify this right now: it can no longer " +
			"read " + string(kind) + " (access was lost)"
	}
	return "kwatch can't verify this right now: reading " +
		string(kind) + " keeps failing"
}
