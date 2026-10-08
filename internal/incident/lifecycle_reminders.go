package incident

import "time"

// Settle windows and reminder timing for open incidents.

// settleFor is how long p collects findings before its first message. A
// page settles fast on its own; in a burst of incidents settling at once
// it waits the full settle, so the cause they share can surface and one
// incident replaces many instead of many being announced and revised.
func (m *Manager) settleFor(p *Incident) time.Duration {
	if p.Tier == Page && m.settlingCount() < m.cfg.BurstIncidents {
		return m.cfg.PageSettle
	}
	return m.cfg.Settle
}

// settlingCount is how many announceable incidents with members are
// settling now; a silent one never speaks, so it joins no burst.
func (m *Manager) settlingCount() int {
	count := 0
	for _, p := range m.incidents {
		if p.State == Settling && len(p.Members) > 0 &&
			p.Tier != Silent {
			count++
		}
	}
	return count
}

// superseded reports whether an announced incident lost every member to
// another announced incident after a cause revision.
func superseded(p *Incident) bool {
	return p.SupersededBy != "" && len(p.Members) == 0 && wasAnnounced(p)
}

// reminderDue reports an announced incident open for another
// RemindEvery since its announcement or its last reminder.
func (m *Manager) reminderDue(p *Incident, now time.Time) bool {
	if acked(p) || idleWebhook(p) {
		return false
	}
	last := p.Reminded
	if last.IsZero() {
		last = p.Announced
	}
	return !last.IsZero() && now.Sub(last) >= reminderEvery(p)
}

// reminderEvery is how long after its announcement or last reminder an
// open incident is said again.
func reminderEvery(p *Incident) time.Duration {
	switch {
	case isPage(p):
		if p.Reminded.IsZero() {
			return PageRemindAfter
		}
		return RemindEvery
	case p.Delivery.RolledUp() || p.Tier == Digest:
		return ChronicRemindEvery
	}
	return RemindEvery
}
