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
			return m.graceDeadline(now)
		}
		settle := m.cfg.Settle
		if p.Tier == Page {
			settle = m.cfg.PageSettle
		}
		return p.Opened.Add(settle), true
	case Open:
		if len(p.Members) == 0 {
			return m.graceDeadline(now)
		}
		if p.revised {
			return p.revisedAt.Add(m.cfg.ReviseSettle), true
		}
	case Recovering:
		if len(p.Members) == 0 {
			hold := m.cfg.hold(len(recent(p.Cycles, now, m.cfg.FlapWindow)))
			return m.afterGrace(p.RecoveringSince.Add(hold), now), true
		}
	case Flapping:
		if len(p.Members) == 0 && !p.RecoveringSince.IsZero() {
			due := p.RecoveringSince.Add(m.cfg.MaxHold)
			return m.afterGrace(due, now), true
		}
	case Resolved:
		return p.Resolved.Add(m.cfg.Remember + time.Nanosecond), true
	}
	return time.Time{}, false
}

func (m *Manager) graceDeadline(now time.Time) (time.Time, bool) {
	return m.restoreGrace, now.Before(m.restoreGrace)
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
	settle := m.cfg.Settle
	if p.Tier == Page {
		settle = m.cfg.PageSettle
	}
	if now.Before(p.Opened.Add(settle)) {
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
		return Decision{}, false
	}
	return m.decide(p, Update, "material change"), true
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
	return now.Before(m.restoreGrace) || !m.verifiable(p)
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
func (m *Manager) afterGrace(at, now time.Time) time.Time {
	if now.Before(m.restoreGrace) && at.Before(m.restoreGrace) {
		return m.restoreGrace
	}
	return at
}

func (m *Manager) verifiable(p *Incident) bool {
	return m.cfg.Verifiable == nil || m.cfg.Verifiable(p.Root.Kind)
}
