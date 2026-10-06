package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Manager methods that record what delivery did with an incident: the
// pipeline reports it here, and the incident keeps it in Delivery (see
// flags.go), Scope, Held and DigestedAt.

// RecordPaged remembers whether the alert of incident id is open at the
// paging providers: a message of its own opens it, so its resolve is sent
// to them too, and that resolve closes it.
func (m *Manager) RecordPaged(id string, open bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil {
		if open {
			p.Delivery.MarkPaged()
		} else {
			p.Delivery.ClosePage()
		}
	}
}

// Paged reports whether the alert of incident id is open at the paging
// providers. An
// unknown incident counts as paged: sending its resolve is the safe
// mistake, a stuck alert is not.
func (m *Manager) Paged(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	return p == nil || p.Delivery.OpenAtPagers()
}

// PageOpen reports whether the alert of incident id is known to be open at
// the paging providers. Unlike Paged, an unknown incident reports false:
// the question here is whether a page still has to be sent, and nothing
// says one was.
func (m *Manager) PageOpen(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	return p != nil && p.Delivery.OpenAtPagers()
}

// RecordRolledUp remembers that a roll-up or the startup summary carried
// incident id's announcement. Its fingerprint then also follows the
// number of failing pods, adopted now so that this does not read as a
// change.
func (m *Manager) RecordRolledUp(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil || p.Delivery.RolledUp() {
		return
	}
	p.Delivery.MarkRolledUp(failingPods(p))
	if p.Digest != "" {
		p.Digest = fingerprint(p)
	}
}

// RecordDigested remembers that a digest listed incident id at at.
func (m *Manager) RecordDigested(id string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil {
		p.DigestedAt = at
	}
}

// failingPods counts the distinct pods with a finding in p.
func failingPods(p *Incident) int {
	pods := map[string]bool{}
	for key, s := range p.Members {
		if key.Entity.Kind == kube.KindPod && !s.Advisory {
			pods[key.Entity.String()] = true
		}
	}
	return len(pods)
}

// notePodPeak raises the pod peak of a rolled-up incident, so the
// fingerprint sees growth and never a dip.
func (p *Incident) notePodPeak() {
	if p.Delivery.RolledUp() {
		p.Delivery.RaisePodPeak(failingPods(p))
	}
}

// RecordScope remembers whether the announcement of incident id was
// delivered. Updates and the resolve of a delivered incident stay in scope
// even after every finding that put it there has cleared.
func (m *Manager) RecordScope(id string, in bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil {
		p.Scope = ScopeOut
		p.sent.withdraw()
		if in {
			p.Scope = ScopeIn
			p.sent.confirm()
		}
	}
}

// HoldAnnouncement marks the announcement of incident id as collected for
// the startup summary. A held incident is persisted as not yet announced,
// so a restart before the summary is sent announces it again.
func (m *Manager) HoldAnnouncement(id string) {
	m.setHeld(id, false, true)
}

// ReleaseAnnouncement marks the held announcement of incident id as handed
// to delivery.
func (m *Manager) ReleaseAnnouncement(id string) {
	m.setHeld(id, true, false)
}

// HoldUpdate marks the latest update of incident id as waiting for its
// investigation. The saved record keeps the fingerprint from before it,
// so a restart that loses the held update decides it again.
func (m *Manager) HoldUpdate(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && p.prevDigest != p.Digest {
		p.updateHeld = true
	}
}

// ReleaseUpdate marks the held update of incident id as handed to
// delivery.
func (m *Manager) ReleaseUpdate(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil {
		p.updateHeld = false
	}
}

// DropAnnouncement clears the held flag of incident id without telling
// anyone: its announcement was never delivered and never will be, because
// the incident resolved first. It stays "nobody heard of it".
func (m *Manager) DropAnnouncement(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil {
		p.Held = false
	}
}

// setHeld moves the held flag. Holding withdraws the announcement's
// sent mark: nobody got it yet. Releasing confirms the latest decision,
// which is delivered right after the release.
func (m *Manager) setHeld(id string, from, to bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && p.Held == from {
		p.Held = to
		if to {
			p.sent.withdraw()
		} else {
			p.sent.confirm()
		}
	}
}

// Closed reports whether none of the incidents ids is still live: each is
// resolved or already forgotten.
func (m *Manager) Closed(ids []string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if p := m.incidents[id]; p != nil && p.State != Resolved {
			return false
		}
	}
	return true
}

// TakeQuietResolves returns, once, the IDs of the incidents that resolved
// without a decision since the last call. Only a resolve can finish a
// startup summary or roll-up, and a quiet one sends no Resolve decision
// to say so, so the announcer asks here whether the listings need a
// check. It also purges what it still holds for those incidents: a
// held announcement would otherwise be sent later for an incident that
// is already over.
func (m *Manager) TakeQuietResolves() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.quietIDs
	m.quietIDs = nil
	return ids
}

// ResolvedQuietly reports whether incident id was resolved without a
// message (see resolveQuietly). A held announcement of such an incident
// must not be sent: the story it would start is told by another one.
func (m *Manager) ResolvedQuietly(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	return p != nil && p.quiet
}
