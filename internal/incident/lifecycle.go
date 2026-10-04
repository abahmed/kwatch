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
		d, ok := m.advance(p, now)
		if ok {
			decisions = append(decisions, d)
		}
		if p.State == Resolved {
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
			hold := m.cfg.hold(len(recent(p.Cycles, now, m.cfg.FlapWindow)))
			return m.afterGrace(p, p.RecoveringSince.Add(hold), now), true
		}
	case Flapping:
		if len(p.Members) == 0 && !p.RecoveringSince.IsZero() {
			due := p.RecoveringSince.Add(m.cfg.MaxHold)
			return m.afterGrace(p, due, now), true
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
	case p.revised:
		return p.revisedAt.Add(m.cfg.ReviseSettle), true
	}
	last := p.Reminded
	if last.IsZero() {
		last = p.Announced
	}
	return last.Add(RemindEvery), !last.IsZero()
}

func (m *Manager) graceDeadline(
	p *Incident, now time.Time,
) (time.Time, bool) {
	return m.restoreGrace, m.inGrace(p, now)
}

func (m *Manager) advance(p *Incident, now time.Time) (Decision, bool) {
	if superseded(p) {
		return m.resolve(p, now, ReasonSuperseded), true
	}
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
		// Recovered before anyone was told: stay silent.
		p.State, p.Resolved = Resolved, now
		p.Fix = fixOf(m.model, p, now)
		return Decision{}, false
	}
	if now.Before(p.Opened.Add(m.settleFor(p))) {
		return Decision{}, false
	}
	if p.Tier == Silent {
		return Decision{}, false
	}
	p.State, p.Announced = Open, now
	if p.AlertKey == "" {
		p.AlertKey = alertKey(p.Root, p.Mode)
	}
	if cycles := len(recent(p.Cycles, now, m.cfg.FlapWindow)); cycles >=
		m.cfg.FlapCycles {
		p.State = Flapping
		p.note(now, "flapping: "+strconv.Itoa(cycles)+
			" recurrences in "+m.cfg.FlapWindow.String())
	}
	return m.decide(p, Announce, "settled"), true
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

// settlingCount is how many incidents with members are settling now.
func (m *Manager) settlingCount() int {
	count := 0
	for _, p := range m.incidents {
		if p.State == Settling && len(p.Members) > 0 {
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

func (m *Manager) open(p *Incident, now time.Time) (Decision, bool) {
	if m.awaitingEvidence(p, now) {
		return Decision{}, false
	}
	if len(p.Members) == 0 {
		p.State, p.RecoveringSince = Recovering, now
		return Decision{}, false
	}
	if p.revised {
		if now.Before(p.revisedAt.Add(m.cfg.ReviseSettle)) {
			// Let the new cause settle, so one update carries it
			// with whatever joins it in the next seconds.
			return Decision{}, false
		}
		p.revised = false
		return m.decide(p, Update, ReasonCauseRevised), true
	}
	if fingerprint(p) == p.Digest {
		if m.reminderDue(p, now) {
			p.Reminded = now
			return m.decide(p, Update, ReasonReminder), true
		}
		return Decision{}, false
	}
	return m.decide(p, Update, ReasonMaterialChange), true
}

// reminderDue reports an announced incident open for another
// RemindEvery since its announcement or its last reminder.
func (m *Manager) reminderDue(p *Incident, now time.Time) bool {
	last := p.Reminded
	if last.IsZero() {
		last = p.Announced
	}
	return !last.IsZero() && now.Sub(last) >= RemindEvery
}

func (m *Manager) recovering(p *Incident, now time.Time) (Decision, bool) {
	if len(p.Members) > 0 {
		// Failed again inside the hold: same incident, no new message.
		p.Cycles = recent(append(p.Cycles, now), now, m.cfg.FlapWindow)
		if len(p.Cycles) >= m.cfg.FlapCycles {
			p.State = Flapping
			p.note(now, "flapping: "+strconv.Itoa(len(p.Cycles))+
				" recoveries in "+m.cfg.FlapWindow.String())
			return m.decide(p, Update, "flapping"), true
		}
		p.State = Open
		return Decision{}, false
	}
	if m.holdRecovery(p, now) {
		return Decision{}, false
	}
	hold := m.cfg.hold(len(recent(p.Cycles, now, m.cfg.FlapWindow)))
	if due := p.RecoveringSince.Add(hold); now.Before(due) {
		return Decision{}, false
	}
	return m.resolve(p, now, "healthy for "+hold.String()), true
}

func (m *Manager) flapping(p *Incident, now time.Time) (Decision, bool) {
	if len(p.Members) > 0 {
		p.RecoveringSince = time.Time{}
		return Decision{}, false
	}
	if p.RecoveringSince.IsZero() {
		p.RecoveringSince = now
	}
	if m.holdRecovery(p, now) {
		return Decision{}, false
	}
	if due := p.RecoveringSince.Add(m.cfg.MaxHold); now.Before(due) {
		return Decision{}, false
	}
	return m.resolve(p, now, "stable for "+m.cfg.MaxHold.String()), true
}

func (m *Manager) resolve(p *Incident, now time.Time, why string) Decision {
	p.State, p.Resolved = Resolved, now
	p.Fix = fixOf(m.model, p, now)
	if why != ReasonSuperseded {
		p.FixedBy = fixingChange(m.model, p, now)
	}
	p.note(now, "resolved: "+why)
	return m.decide(p, Resolve, why)
}

func (m *Manager) decide(p *Incident, action Action, why string) Decision {
	p.Revision++
	p.Digest = fingerprint(p)
	p.movedTo = nil
	d := Decision{Action: action, Incident: p.Snapshot(), Reason: why}
	p.sent.decided(p.Scope != ScopeOut && !p.Held)
	return d
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

// awaitingEvidence reports whether an incident without members must keep
// its state: during the restore grace detectors may not have re-raised
// its findings yet, and a root kind kwatch cannot observe says nothing
// about recovery.
func (m *Manager) awaitingEvidence(p *Incident, now time.Time) bool {
	if len(p.Members) > 0 {
		return false
	}
	return m.inGrace(p, now) || !m.verifiable(p)
}

// inGrace reports whether p is a restored incident still inside the
// restore grace. Incidents opened after the restart have no stale
// model to wait for: their members come from this session's detectors.
func (m *Manager) inGrace(p *Incident, now time.Time) bool {
	return p.restored && now.Before(m.restoreGrace)
}

// holdRecovery reports whether a recovering incident without members
// must not resolve yet. During the restore grace the model is still being
// rebuilt, so missing members say nothing; the hold keeps running. A root
// kwatch cannot observe restarts the hold instead of resolving on missing
// data.
func (m *Manager) holdRecovery(p *Incident, now time.Time) bool {
	if !m.awaitingEvidence(p, now) {
		return false
	}
	if !m.verifiable(p) {
		p.RecoveringSince = now
	}
	return true
}

// afterGrace moves a deadline that falls inside the restore grace to the
// end of the grace, when recovery may first be decided.
func (m *Manager) afterGrace(
	p *Incident, at, now time.Time,
) time.Time {
	if m.inGrace(p, now) && at.Before(m.restoreGrace) {
		return m.restoreGrace
	}
	return at
}

func (m *Manager) verifiable(p *Incident) bool {
	return m.cfg.Verifiable == nil || m.cfg.Verifiable(p.Root.Kind)
}
