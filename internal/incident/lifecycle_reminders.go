package incident

import "time"

// Settle windows and reminder timing for open incidents.

// settleFor is how long p collects findings before its first message. A
// page settles fast on its own; in a burst of incidents settling at once
// it waits the full settle, so the cause they share can surface and one
// incident replaces many instead of many being announced and revised.
// A failure that kwatch found already there at its start takes the full
// settle too: it may be clearing already, and an urgent page is for news.
// A page held for a scaling wave settles until the wave is over
// (scale_hold.go).
func (m *Manager) settleFor(p *Incident) time.Duration {
	settle := m.cfg.Settle
	if m.fastPage(p) {
		settle = m.cfg.PageSettle
	}
	return m.scaleSettle(p, settle)
}

// fastPage reports whether p settles with the short page settle.
func (m *Manager) fastPage(p *Incident) bool {
	return p.Tier == Page && m.settlingCount() < m.cfg.BurstIncidents &&
		!m.foundAtStart(p)
}

// startFindSlack is how long after kwatch's first look an incident may
// open and still count as one that existed when kwatch started: the
// first sync of the sources is not instant.
const startFindSlack = 30 * time.Second

// foundAtStart reports an incident opened by kwatch's first look at the
// cluster with a failing member whose onset is older than kwatch.
func (m *Manager) foundAtStart(p *Incident) bool {
	if m.started.IsZero() || p.Opened.After(m.started.Add(startFindSlack)) {
		return false
	}
	for _, f := range p.Members {
		if !f.Since.IsZero() && f.Since.Before(m.started) {
			return true
		}
	}
	return false
}

// settlingCount is how many announceable incidents with members are
// settling now; a silent one never speaks, so it joins no burst.
func (m *Manager) settlingCount() int {
	count := 0
	for _, p := range m.incidents {
		if p.State == Settling && len(p.Members) > 0 &&
			p.Tier >= Notify {
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
	if m.cfg.NoReminders || acked(p) || idleWebhook(p) {
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
		return chronicBackoff(p.Reminders)
	}
	return RemindEvery
}
