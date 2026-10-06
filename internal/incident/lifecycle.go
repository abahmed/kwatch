package incident

import (
	"sort"
	"strconv"
	"time"
)

// Tick advances every incident's lifecycle and returns the decisions that
// warrant a message, plus the delay until the earliest pending deadline
// computed from the state after all transitions (zero when none).
func (m *Manager) Tick(now time.Time) ([]Decision, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var decisions []Decision
	m.adoptFingerprints(now)
	// Only live incidents advance; resolved ones only wait to expire.
	for _, id := range m.liveIDs() {
		p := m.incidents[id]
		if p.restored && p.bootedAt.IsZero() {
			p.bootedAt = now
		}
		d, ok := m.advance(p, now)
		if ok {
			decisions = append(decisions, d)
		}
		if p.State == Resolved && m.incidents[id] == p {
			m.markResolved(p)
		}
	}
	m.expireResolved(now)
	return decisions, m.nextWake(now)
}

// adoptFingerprints stores the current fingerprint of each restored
// announced incident whose members are back, before Tick compares
// fingerprints. The stored one may come from an older kwatch whose
// formula or fields differed; comparing it would send an update about
// nothing after every upgrade. Members keep re-attaching during the
// restore grace, so the fingerprint is adopted on every tick until the
// grace is over, and then
// once more when the incident has members. Resolved and forgotten
// incidents leave the list.
func (m *Manager) adoptFingerprints(now time.Time) {
	if len(m.refingerprint) == 0 {
		return
	}
	graceOver := !now.Before(m.restoreGrace)
	kept := m.refingerprint[:0]
	for _, id := range m.refingerprint {
		p := m.incidents[id]
		if p == nil || !wasAnnounced(p) {
			continue
		}
		if len(p.Members) == 0 {
			kept = append(kept, id)
			continue
		}
		p.Digest = adoptedFingerprint(p)
		if !graceOver {
			kept = append(kept, id)
		}
	}
	m.refingerprint = kept
}

// adoptedFingerprint fingerprints a recovering incident as open: members
// are back, so it reopens without a decision, and a digest that kept the
// recovering state would read as a change on the next tick.
func adoptedFingerprint(p *Incident) string {
	if p.State != Recovering {
		return fingerprint(p)
	}
	reopened := *p
	reopened.State = Open
	return fingerprint(&reopened)
}

// advance moves one live incident through one tick. The order is:
// superseded (the incident lost every member to another one), reopened
// (its "failing again" update is due), escalation (a tier or state change
// without a message), then the handler of its state.
func (m *Manager) advance(p *Incident, now time.Time) (Decision, bool) {
	if superseded(p) {
		if m.quietSupersede(p, now) {
			m.resolveQuietly(p, now)
			return Decision{}, false
		}
		return m.resolve(p, now, ReasonSuperseded), true
	}
	if m.reopenPending(p) {
		return m.reopenUpdate(p, now)
	}
	m.escalate(p, now)
	switch p.State {
	case Settling:
		return m.settle(p, now)
	case Open:
		return m.open(p, now)
	case Recovering:
		return m.recovering(p, now)
	case Flapping:
		return m.flapping(p, now)
	}
	return Decision{}, false
}

func (m *Manager) settle(p *Incident, now time.Time) (Decision, bool) {
	if m.awaitingEvidence(p, now) {
		return Decision{}, false
	}
	if len(p.Members) == 0 {
		// Recovered before anyone was told: stay silent, unless a page
		// reached the pagers first (a held page restored after a
		// restart): its alert is closed, and only that.
		p.State, p.Resolved = Resolved, now
		p.Fix = fixOf(m.model, p, now)
		if p.Delivery.OpenAtPagers() {
			return m.closeUnheardPage(p, now), true
		}
		return Decision{}, false
	}
	if now.Before(p.Opened.Add(m.settleFor(p))) {
		return Decision{}, false
	}
	if p.Tier == Silent {
		return Decision{}, false
	}
	p.State, p.Announced = Open, now
	m.holdRepeatedPage(p, now)
	if p.AlertKey == "" {
		p.AlertKey = m.freeAlertKey(p)
	}
	if cycles := len(recent(p.Cycles, now, m.cfg.FlapWindow)); cycles >=
		m.cfg.FlapCycles {
		m.enterFlapping(p, now, cycles, "recurrences")
	}
	return m.decide(p, Announce, ReasonSettled), true
}

// closeUnheardPage is the resolve of an incident that paged while its
// announcement was held and then recovered before anyone but the pagers
// heard of it. It is not recorded as told, so the incident stays one
// nobody can reopen, and it carries Unannounced so that only the pagers
// get it (the same as closing a held page in the startup summary).
func (m *Manager) closeUnheardPage(p *Incident, now time.Time) Decision {
	p.Pending.ClearReopen()
	p.Pending.ClearRevised()
	p.note(now, "resolved: "+string(ReasonRecoveredUnheard))
	return Decision{Action: Resolve, Incident: p.Snapshot(),
		Reason: ReasonRecoveredUnheard, Unannounced: true}
}

// holdRepeatedPage keeps a page that re-opens soon after a page resolved
// from paging again: the outage is the same, and a fourth page in two
// hours tells no one more than the first. The incident notifies, says it
// failed again, and keeps the page's reminders.
func (m *Manager) holdRepeatedPage(p *Incident, now time.Time) {
	if p.Tier != Page {
		return
	}
	times, repeat := repeatsPage(p, now)
	if !repeat {
		return
	}
	p.Delivery.HoldAtNotify()
	p.Tier = Notify
	p.note(now, repeatNote(times))
}

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

func (m *Manager) recovering(p *Incident, now time.Time) (Decision, bool) {
	if len(p.Members) > 0 && (m.inGrace(p, now) || !m.verifiable(p)) {
		// A restored incident whose findings are coming back: the
		// failure never stopped, the restart only hid it. A root kind
		// kwatch cannot observe never proved a recovery either. No cycle.
		p.State = Open
		return Decision{}, false
	}
	if len(p.Members) > 0 {
		// Failed again inside the hold: same incident, no new message.
		p.Cycles = recent(append(p.Cycles, now), now, m.cfg.FlapWindow)
		if len(p.Cycles) >= m.cfg.FlapCycles {
			m.enterFlapping(p, now, len(p.Cycles), "recoveries")
			return m.decide(p, Update, ReasonFlapping), true
		}
		p.State = Open
		return Decision{}, false
	}
	if m.holdRecovery(p, now) {
		return Decision{}, false
	}
	hold := m.holdFor(p, now)
	if due := p.RecoveringSince.Add(hold); now.Before(due) ||
		m.stillBroken(p, now) {
		return Decision{}, false
	}
	healthy := Reason("healthy for " + hold.String())
	return m.resolve(p, now, m.resolveReason(p, healthy)), true
}

// enterFlapping turns p into a flapping incident and notes how many
// cycles (named by what, "recurrences" or "recoveries") put it there.
func (m *Manager) enterFlapping(
	p *Incident, now time.Time, cycles int, what string,
) {
	p.State = Flapping
	p.note(now, "flapping: "+strconv.Itoa(cycles)+" "+what+" in "+
		m.cfg.FlapWindow.String())
}

func (m *Manager) flapping(p *Incident, now time.Time) (Decision, bool) {
	if len(p.Members) > 0 {
		p.RecoveringSince = time.Time{}
		return m.flappingNews(p, now)
	}
	if p.RecoveringSince.IsZero() {
		p.RecoveringSince = now
	}
	if m.holdRecovery(p, now) {
		return Decision{}, false
	}
	if due := p.RecoveringSince.Add(m.cfg.MaxHold); now.Before(due) ||
		m.stillBroken(p, now) {
		return Decision{}, false
	}
	stable := Reason("stable for " + m.cfg.MaxHold.String())
	return m.resolve(p, now, m.resolveReason(p, stable)), true
}

// flappingNews is what a flapping incident with failing members may say:
// that the failure grew, or, for an announced incident, its reminder.
func (m *Manager) flappingNews(p *Incident, now time.Time) (Decision, bool) {
	if flapGrew(p) {
		return m.decide(p, Update, ReasonMaterialChange), true
	}
	if wasAnnounced(p) && m.reminderDue(p, now) {
		p.Reminded = now
		return m.decide(p, Update, ReasonReminder), true
	}
	return Decision{}, false
}

func (m *Manager) resolve(p *Incident, now time.Time, why Reason) Decision {
	p.State, p.Resolved = Resolved, now
	p.Pending.ClearReopen()
	p.Pending.ClearRevised()
	p.Fix = fixOf(m.model, p, now)
	if why != ReasonSuperseded {
		p.FixedBy = fixingChange(m.model, p, now)
	}
	p.note(now, "resolved: "+string(why))
	d := m.decide(p, Resolve, why)
	m.reform(p, now)
	return d
}

// resolveQuietly closes p without a message: its failures live on in
// another incident that already tells the story.
func (m *Manager) resolveQuietly(p *Incident, now time.Time) {
	p.State, p.Resolved, p.quiet = Resolved, now, true
	p.Held = false
	p.Pending.ClearReopen()
	p.Pending.ClearRevised()
	m.quietIDs = append(m.quietIDs, p.ID)
	p.Fix = fixOf(m.model, p, now)
	p.note(now, "resolved: "+string(ReasonSuperseded))
}

func (m *Manager) decide(p *Incident, action Action, why Reason) Decision {
	p.Revision++
	p.prevDigest, p.updateHeld = p.Digest, false
	p.Digest = fingerprint(p)
	p.Delivery.RecordSent(p.Tier, growthKey(p))
	p.decided = failingKeys(p)
	p.movedTo = nil
	delivered := p.Scope != ScopeOut && !p.Held
	p.noteRoute(action, delivered)
	d := Decision{Action: action, Incident: p.Snapshot(), Reason: why}
	// An announcement of an incident whose alert is already open (a
	// restored held page) must not page again.
	d.PagedAlready = action == Announce && p.Delivery.OpenAtPagers()
	p.sent.decided(delivered)
	return d
}

// noteRoute keeps AnnouncedRoute: the announcement records it, and each
// update that is sent widens it. A resolve never changes it.
func (p *Incident) noteRoute(action Action, delivered bool) {
	switch {
	case action == Announce && p.AnnouncedRoute == nil:
		p.AnnouncedRoute = routeOf(p)
	case action == Update && delivered && p.AnnouncedRoute != nil:
		p.AnnouncedRoute.widen(routeOf(p))
	}
}

func (m *Manager) sortedIDs() []string {
	ids := make([]string, 0, len(m.incidents))
	for id := range m.incidents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func recent(
	times []time.Time, now time.Time, window time.Duration,
) []time.Time {
	out := times[:0:0]
	for _, t := range times {
		if now.Sub(t) <= window {
			out = append(out, t)
		}
	}
	return out
}

func uniq(sorted []string) []string {
	out := sorted[:0:0]
	for i, v := range sorted {
		if i == 0 || sorted[i-1] != v {
			out = append(out, v)
		}
	}
	return out
}
