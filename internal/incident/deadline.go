package incident

import "time"

// Timers: when each incident next needs a tick on its own.

// nextWake returns the delay to the earliest future deadline over all
// incidents, or zero when no timer is pending.
func (m *Manager) nextWake(now time.Time) time.Duration {
	var next time.Duration
	consider := func(at time.Time) {
		if delay := at.Sub(now); delay > 0 && (next == 0 || delay < next) {
			next = delay
		}
	}
	if p := m.oldestResolved(); p != nil {
		consider(p.Resolved.Add(m.cfg.Remember + time.Nanosecond))
	}
	for _, p := range m.incidents {
		if p.State == Resolved {
			continue
		}
		at, ok := m.deadline(p, now)
		if ok {
			consider(at)
		}
	}
	return next
}

// deadline reports when the incident next needs a tick on its own.
func (m *Manager) deadline(p *Incident, now time.Time) (time.Time, bool) {
	if m.reopenPending(p) {
		return p.Pending.ReopenDueAt(m.cfg.ReviseSettle), true
	}
	switch p.State {
	case Settling:
		if len(p.Members) == 0 {
			return m.graceDeadline(p, now)
		}
		return p.Opened.Add(m.settleFor(p)), true
	case Open:
		return m.openDeadline(p, now)
	case Recovering:
		if len(p.Members) == 0 {
			due := p.RecoveringSince.Add(m.holdFor(p, now))
			return m.afterGrace(p, m.holdEnd(p, due, now), now), true
		}
	case Flapping:
		if len(p.Members) > 0 {
			return m.reminderDeadline(p)
		}
		if !p.RecoveringSince.IsZero() {
			due := p.RecoveringSince.Add(m.cfg.MaxHold)
			return m.afterGrace(p, m.holdEnd(p, due, now), now), true
		}
	case Resolved:
		return p.Resolved.Add(m.cfg.Remember + time.Nanosecond), true
	}
	return time.Time{}, false
}

// openDeadline is the next timer of an open incident: its revision
// settle, or else its weekly reminder.
func (m *Manager) openDeadline(
	p *Incident, now time.Time,
) (time.Time, bool) {
	switch {
	case len(p.Members) == 0:
		return m.graceDeadline(p, now)
	case p.Pending.RevisedOwed():
		return p.Pending.RevisedDueAt(m.cfg.ReviseSettle), true
	}
	due, ok := m.reminderDeadline(p)
	low, lowOK := m.reassessDeadline(p, now)
	due, ok = soonest(due, ok, low, lowOK)
	if p.Attempt != nil && !p.attemptLate {
		due = earliest(due, p.Attempt.At.Add(FixWatch))
	}
	esc, escOK := escalationDeadline(p)
	due, ok = soonest(due, ok, esc, escOK)
	held, heldOK := heldDeadline(p, now)
	return soonest(due, ok, held, heldOK)
}

// heldDeadline is when a material change held back by MaterialGap may
// be sent, while one is waiting.
func heldDeadline(p *Incident, now time.Time) (time.Time, bool) {
	last := p.Delivery.LastMaterial()
	if last.IsZero() || !now.Before(last.Add(MaterialGap)) ||
		fingerprint(p) == p.Digest {
		return time.Time{}, false
	}
	return last.Add(MaterialGap), true
}

// soonest merges two optional deadlines into the earlier one.
func soonest(
	a time.Time, aok bool, b time.Time, bok bool,
) (time.Time, bool) {
	switch {
	case !aok:
		return b, bok
	case !bok:
		return a, true
	}
	return earliest(a, b), true
}

// reminderDeadline is when an announced incident is next reminded of.
func (m *Manager) reminderDeadline(p *Incident) (time.Time, bool) {
	last := p.Reminded
	if last.IsZero() {
		last = p.Announced
	}
	return last.Add(reminderEvery(p)), !last.IsZero() && !acked(p)
}

// earliest is the earlier of two times.
func earliest(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func (m *Manager) graceDeadline(
	p *Incident, now time.Time,
) (time.Time, bool) {
	return m.restoreGrace, m.inGrace(p, now)
}
