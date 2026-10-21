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
// resolved or already forgotten, and no incident that took over its
// failures (a superseding one, or the recurrence of a forgotten one) is
// live either.
func (m *Manager) Closed(ids []string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	var recurrences map[string]string
	for _, id := range ids {
		if !m.closedLocked(id, 0, &recurrences) {
			return false
		}
	}
	return true
}

// maxFollowHops bounds the walk along superseding incidents and
// recurrences.
const maxFollowHops = 8

// closedLocked reports an incident that is over: resolved, or forgotten,
// and, when it is, no live successor. A superseded incident resolved
// because its failures moved, not because they stopped; a forgotten one
// was replaced by the recurrence that carries its history (Previous).
// recurrences maps a forgotten ID to the incident that recurred from it,
// built on first use.
func (m *Manager) closedLocked(
	id string, hops int, recurrences *map[string]string,
) bool {
	next, over := m.successorOf(id, recurrences)
	if !over {
		return false
	}
	return next == "" || hops >= maxFollowHops ||
		m.closedLocked(next, hops+1, recurrences)
}

// successorOf reports whether incident id is over and the incident that
// carries its story on, empty when none does.
func (m *Manager) successorOf(
	id string, recurrences *map[string]string,
) (next string, over bool) {
	p := m.incidents[id]
	switch {
	case p == nil:
		if *recurrences == nil {
			*recurrences = m.recurrenceIndex()
		}
		return (*recurrences)[id], true
	case p.State != Resolved:
		return "", false
	}
	return p.SupersededBy, true
}

// recurrenceIndex maps the ID of each forgotten incident that has a
// successor to the ID of that successor.
func (m *Manager) recurrenceIndex() map[string]string {
	out := map[string]string{}
	for id, p := range m.incidents {
		if p.Previous != "" && m.incidents[p.Previous] == nil {
			out[p.Previous] = id
		}
	}
	return out
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

// HoldPage keeps incident id from paging: a page for the problems it was
// announced with already went out. It notifies instead and keeps the
// page's reminders, as a repeated page does; it pages again only when it
// outlasts the hold (see releaseHeldPage).
func (m *Manager) HoldPage(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && p.Tier == Page &&
		!p.Delivery.OpenAtPagers() && !p.Delivery.PageHeld() {
		unchanged := p.Digest == fingerprint(p)
		p.Delivery.HoldAtNotify()
		p.Tier = Notify
		p.note(p.Announced, "announced with a page for the same moment")
		if unchanged {
			// The hold is no news: the listing already told the
			// problem, and an update for the tier would repeat it.
			p.Digest = fingerprint(p)
		}
	}
}
