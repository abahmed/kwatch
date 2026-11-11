package incident

import "time"

// A repeated page is held at notify so a flapping outage pages once
// (see holdRepeatedPage). The hold must not outlive the flap: a failure
// that stays down for as long as a recovery needs to be believed is not
// a flap, so it pages once more.

// heldPageDue is when a held repeat that keeps failing pages again: the
// page settle plus the base hold a first recovery waits, counted from its
// return. The doubled holds of a flapper do not apply: they say how long
// a flapper must be healthy, not how long a dead node may stay unpaged.
func (m *Manager) heldPageDue(p *Incident) time.Time {
	return lastReturn(p).Add(m.cfg.PageSettle + m.cfg.Hold)
}

// lastReturn is when p's failure last came back (Cycles), or when it
// opened.
func lastReturn(p *Incident) time.Time {
	at := p.Opened
	if n := len(p.Cycles); n > 0 && p.Cycles[n-1].After(at) {
		at = p.Cycles[n-1]
	}
	return at
}

// failingSince reports an open or flapping incident that has not
// recovered since its last return: a recovery in between would have
// started RecoveringSince, and a return after it a new cycle.
func failingSince(p *Incident) bool {
	return (p.State == Open || p.State == Flapping) &&
		len(p.Members) > 0 && p.RecoveringSince.IsZero()
}

// holdsPage reports a held repeat whose page is still due. Someone who
// acknowledged the incident knows about it, so it does not page; nor does
// one whose alert is open at the pagers already (see pagesFlapper).
func (m *Manager) holdsPage(p *Incident) bool {
	if !p.Delivery.PageHeld() || p.Tier == Page ||
		p.Delivery.OpenAtPagers() {
		return false
	}
	return !m.pageAcknowledged(p)
}

// pageAcknowledged reports an incident someone is already on: the thread
// was told of the acknowledgement, or the annotation is on the root or a
// member and kwatch has not told yet. Paging it would wake the people who
// are looking at it.
func (m *Manager) pageAcknowledged(p *Incident) bool {
	if acked(p) {
		return true
	}
	_, annotated := m.ackOn(p)
	return annotated
}

// pagedBeside reports another live incident whose page is open and that
// was announced with this one, or is of its story: the outage was paged,
// and this incident is its other half.
func (m *Manager) pagedBeside(p *Incident) bool {
	for _, q := range m.incidents {
		if q == p || q.State == Resolved || !q.Delivery.OpenAtPagers() {
			continue
		}
		if (p.Root.Namespace == q.Root.Namespace &&
			absGap(p.Announced, q.Announced) <= m.cfg.Settle) ||
			m.model != nil && sameChain(m.model, q.Root, p.Root) {
			return true
		}
	}
	return false
}

// releaseHeldPage ends the hold of a repeated page that kept failing
// past heldPageDue, when the members still call for a page. Only the
// tier changes; the next step sends the update that pages.
func (m *Manager) releaseHeldPage(p *Incident, now time.Time) {
	if !m.holdsPage(p) || !failingSince(p) ||
		now.Before(m.heldPageDue(p)) || m.pagedBeside(p) {
		return
	}
	p.Delivery.ReleasePageHold()
	if next := m.override.apply(p, tier(p)); next == Page {
		p.Tier = next
		p.note(now, "still failing: paging again")
		return
	}
	p.Delivery.HoldAtNotify()
}

// heldPageDeadline is when releaseHeldPage should next run, while a held
// repeat is failing.
func (m *Manager) heldPageDeadline(p *Incident) (time.Time, bool) {
	if !m.holdsPage(p) || !failingSince(p) {
		return time.Time{}, false
	}
	return m.heldPageDue(p), true
}
