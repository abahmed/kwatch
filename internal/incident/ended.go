package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// RecordSubject remembers what the first message of incident id was
// about. Only the first call counts: the resolve names the subject people
// were told about, not whatever the root became.
func (m *Manager) RecordSubject(id string, subject inventory.EntityID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && p.Subject == (inventory.EntityID{}) &&
		p.AnnouncedRoute != nil {
		p.Subject = subject
	}
}

// FailedFor is how long the incident was failing: from opening to the
// moment its last failure cleared. The resolve hold that follows is
// waiting, not failing, so it is left out.
func (p Incident) FailedFor() time.Duration {
	end := p.Resolved
	if r := p.RecoveringSince; r.After(p.Opened) && !r.After(end) {
		end = r
	}
	return end.Sub(p.Opened)
}

// noteRemoved records which of the incident's subjects were deleted
// while it was open, so its resolve says so rather than claiming a
// recovery.
func (m *Manager) noteRemoved(p *Incident) {
	p.Removed, p.NamespaceRemoved = nil, false
	if m.model == nil {
		return
	}
	for _, id := range []inventory.EntityID{p.Root, p.Subject} {
		if id != (inventory.EntityID{}) && !slices.Contains(p.Removed, id) &&
			removedSince(m.model, id, p.Opened) {
			p.Removed = append(p.Removed, id)
		}
	}
	if ns := p.Root.Namespace; ns != "" {
		p.NamespaceRemoved = removedSince(m.model,
			inventory.CoreID(kube.KindNamespace, "", ns), p.Opened)
	}
}

// removedSince reports an entity that was deleted after since and is
// still gone.
func removedSince(
	model inventory.Reader, id inventory.EntityID, since time.Time,
) bool {
	if model.Exists(id) {
		return false
	}
	for _, change := range model.Changes(id, since) {
		if change.Deleted && !change.At.Before(since) {
			return true
		}
	}
	return false
}
