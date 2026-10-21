package incident

import "time"

// A failing probe or a restarting pod may raise and clear its finding
// every few seconds, as a cluster does while it wakes up. An incident
// that recovers while settling is dropped silently, so each raise used to
// open a new one: a new ID every few seconds for one failing root, none
// of them ever told. The silent one is revived instead: same ID, settling
// again, with the recurrence bookkeeping of a new incident (flap cycles,
// occurrences, the earlier blip in its history).

// revivable reports a resolved incident nobody ever heard of that the same
// root failed again within FlapWindow of its silent end: it recovered
// while settling, so no message, no held announcement and no alert of it
// exists.
func (m *Manager) revivable(p *Incident, now time.Time) bool {
	return p.State == Resolved && p.Announced.IsZero() && !p.sent.told &&
		!p.Held && !p.quiet && p.SupersededBy == "" &&
		len(p.Members) == 0 && !p.Delivery.OpenAtPagers() &&
		now.Sub(p.Resolved) <= m.cfg.FlapWindow
}

// revive makes the silently resolved incident p settle again under its
// own ID.
func (m *Manager) revive(p *Incident, now time.Time) {
	m.dropResolved(p.ID, p.Root.String())
	p.History = appendHistory(p.History, occurrenceOf(p))
	p.Cycles = recent(p.Cycles, now, m.cfg.FlapWindow)
	p.Cycles = append(p.Cycles, now)
	p.Occurrences = appendOccurrence(p.Occurrences, now)
	week := recent(p.Occurrences, now, recurrenceWeek)
	p.note(now, "happened again ("+ordinal(len(week))+" time this week)")
	p.State, p.Opened, p.Resolved = Settling, now, time.Time{}
	p.RecoveringSince = time.Time{}
	p.Fix, p.FixedBy = "", nil
	m.noteOpened(p.ID)
	m.changed[p.ID] = true
}
