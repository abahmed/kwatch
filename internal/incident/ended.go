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
	return max(0, p.RecoveredAt().Sub(p.Opened))
}

// RecoveredAt is the moment the incident's last failure cleared, the
// moment a fix is measured against: when it began recovering, not when
// the resolve hold that followed ended.
func (p Incident) RecoveredAt() time.Time {
	end := p.Resolved
	if r := p.RecoveringSince; r.After(p.Opened) && !r.After(end) {
		end = r
	}
	return end
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
			(removedSince(m.model, id, p.Opened) || m.goneUnseen(p, id)) {
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

// goneUnseen reports a root that failed by itself, so it existed, and
// that a fully listed model no longer holds although no deletion was
// seen after the opening: it was deleted while kwatch was down, and the
// deletion found at startup is dated at the older snapshot, or not
// found at all. A root that never existed (a Service an Ingress routes
// to) has no reason of its own and is not claimed removed.
func (m *Manager) goneUnseen(p *Incident, id inventory.EntityID) bool {
	return id == p.Root && p.restored && len(p.rootReasons) > 0 &&
		!m.model.Exists(id) && m.rootVerifiable(id)
}
