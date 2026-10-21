package incident

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// selfRow names the explain row of a workload blamed for itself.
const selfRow = "self"

// Standalone reports an incident that nothing ties to another failure:
// no cause was found, or the workload is blamed for itself. Many of them
// in one namespace at one time are a namespace outage.
func (p *Incident) Standalone() bool {
	return p.Cause == nil || p.Cause.Rule == selfRow
}

// placement is where the findings of one failure belong: the root of
// their incident, the cause that explains them and what kwatch could
// not check on the way.
type placement struct {
	root       inventory.EntityID
	cause      *rootcause.CauseRecord
	unverified []string
	// checked lists the kinds upstream of the failure that were found
	// healthy and unchanged; see Incident.Checked.
	checked []string
	// considered lists the other causes above the floor, best first,
	// as "root (row, confidence)"; see Incident.Considered.
	considered []string
	// unclear is set, with no cause, when candidates outside root's
	// own workload were considered and dropped.
	unclear bool
	// outscored lists the roots of the alternatives that explain the
	// failures through the same row as cause and score at least as high.
	outscored []inventory.EntityID
}

// placeArea moves the findings of every failure of a solved area to
// the incident of the cause that covers it. A storm puts a thousand
// failures under one cause, so the cause covering each failure and the
// record of each cause are looked up once per area, not per failure.
func (m *Manager) placeArea(
	s explain.Snapshot, area explain.Area, placed map[detection.Key]bool,
) {
	p := newAreaPlacer(s, area)
	for _, failure := range area.Failures {
		where := p.placementOf(failure)
		for _, f := range s.Findings[failure] {
			if f.Advisory {
				// A configuration risk follows no cause; it joins the
				// incident rooted at its own object (adoptAdvisories).
				continue
			}
			m.attach(s.Now, f, where)
			placed[f.Key()] = true
		}
	}
}

// attach makes s a member of the incident of where.root. A finding that
// belonged to another incident is moved: its cause was revised.
func (m *Manager) attach(
	now time.Time, s detection.Finding, where placement,
) {
	if s.Advisory && !m.hasFailure(where.root) {
		// A configuration risk is not an incident. It joins the
		// incident of a real failure of the same root, if there is one.
		return
	}
	if m.heldForChurn(now, s, where) || m.leavesAsHistory(now, s, where) {
		return
	}
	key := s.Key()
	where = m.reroute(now, s, where)
	revised, rerooted := false, false
	if previous, ok := m.byMember[key]; ok && !m.holds(previous, where.root) {
		if m.keepsMember(now, previous, s, where) {
			return
		}
		revised = true
		rerooted = m.revise(now, s, previous, where.root)
	}
	p := m.incident(now, where.root, s)
	m.inheritPage(now, p, s)
	if revised && !rerooted {
		p.note(now, "cause revised: now explained by "+describe(where.root))
	}
	if _, known := p.Members[key]; !known {
		about := s.Entity
		p.noteAbout(now, s.Summary+" ("+describe(s.Entity)+")", &about)
	}
	p.Members[key] = s
	logMember("added to", p, key)
	p.SupersededBy, p.SupersededRoot = "", inventory.EntityID{}
	p.takeCause(where)
	p.Unverified = rootcause.MergeUnverified(p.Unverified, where.unverified)
	for _, kind := range where.checked {
		p.Checked = mergeSorted(p.Checked, kind)
	}
	if len(where.considered) > 0 {
		p.Considered = where.considered
	}
	m.byMember[key] = p.ID
	m.changed[p.ID] = true
}

// keepsMember keeps finding s in its incident, and says so, when moving
// it would only be noise. What people were told about long ago was not
// caused by something that began this morning: the new cause explains
// only the new failures. And the same failures blamed on another object
// of the same workload are not news either: the root stays as announced.
// A failure whose evidence merely aged out is not a new problem while
// the incident's root still fails.
func (m *Manager) keepsMember(
	now time.Time, previous string, s detection.Finding, where placement,
) bool {
	key := s.Key()
	if !m.staysInGroup(previous, where) &&
		!m.edgesOutIncumbent(now, previous, where) &&
		!m.beganTooLate(previous, where.cause) &&
		!m.sameChainFlip(previous, key, where.root) &&
		!m.explanationLapsed(previous, where) {
		return false
	}
	m.incidents[previous].Members[key] = s
	return true
}

// ratchetTier keeps an announced incident at the loudest tier it reached.
// Members of a crash loop come and go within seconds, and a tier that
// followed them would flip between page and notify, each flip a
// "material change". Escalation is news; de-escalation is told by the
// recovery. Before the first message the tier still follows the members.
func (m *Manager) ratchetTier(p *Incident, computed Tier) Tier {
	if p.Announced.IsZero() || computed >= p.Tier || unpaged(p) {
		return computed
	}
	return p.Tier
}

// refresh recomputes what depends on the whole member set of every
// incident that changed in this Apply: mode, impact and tier. Doing it
// once per incident keeps a storm of members linear.
func (m *Manager) refresh(model inventory.Reader, now time.Time) {
	for id := range m.changed {
		if p := m.incidents[id]; p != nil {
			p.Mode = incidentMode(p)
			p.Impact = impact(model, p)
			p.trafficLost = trafficLost(model, p)
			p.servingDown = servingDown(model, p)
			p.admissionBlocked = admissionBlocked(model, p)
			p.noteRejection(now)
			p.routedMissing = routedMissing(model, p)
			p.impactPeak = max(p.impactPeak, impactSize(p))
			p.rememberRootReasons()
			p.noteStage()
			p.noteModes()
			p.notePodPeak()
			p.causeChain = m.inChain(p)
			p.Tier = m.ratchetTier(p, m.override.apply(p, tier(p)))
		}
	}
	clear(m.changed)
}

// wasAnnounced reports whether people were told about p and it is not
// closed yet.
func wasAnnounced(p *Incident) bool {
	switch p.State {
	case Open, Recovering, Flapping:
		return true
	}
	return false
}

func (m *Manager) detach(now time.Time, s detection.Finding) {
	key := s.Key()
	delete(m.churn.held, key)
	delete(m.left, key)
	id, ok := m.byMember[key]
	if !ok {
		return
	}
	delete(m.byMember, key)
	p := m.incidents[id]
	if p == nil {
		return
	}
	delete(p.Members, key)
	logMember("removed from", p, key)
	about := s.Entity
	p.noteAbout(now,
		RecoveredPrefix+s.Summary+" ("+describe(s.Entity)+")", &about)
	if !s.Advisory && !hasFailingMember(p) {
		m.dropAdvisories(p)
	}
}

// hasFailure reports whether root has a live incident with a real
// failure among its members.
func (m *Manager) hasFailure(root inventory.EntityID) bool {
	p := m.lookup(root)
	return p != nil && p.State != Resolved && hasFailingMember(p)
}

// hasFailingMember reports whether any member of p is a failure rather
// than a configuration risk.
func hasFailingMember(p *Incident) bool {
	for _, s := range p.Members {
		if !s.Advisory {
			return true
		}
	}
	return false
}

// dropAdvisories removes the configuration risks from an incident whose
// last failure recovered, so the incident recovers too. The risks stay
// active findings and join the next failure of the same root.
func (m *Manager) dropAdvisories(p *Incident) {
	for key, s := range p.Members {
		if s.Advisory {
			delete(p.Members, key)
			delete(m.byMember, key)
		}
	}
	m.changed[p.ID] = true
}

// adoptAdvisories adds the active configuration risks of each changed
// incident's root to its members, when the incident has a real failure:
// the message then says what the risk cost.
func (m *Manager) adoptAdvisories(s explain.Snapshot) {
	for id := range m.changed {
		p := m.incidents[id]
		if p == nil || !hasFailingMember(p) {
			continue
		}
		for _, f := range s.Findings[p.Root] {
			key := f.Key()
			if _, ok := p.Members[key]; !f.Advisory || ok {
				continue
			}
			p.Members[key] = f
			m.byMember[key] = p.ID
		}
	}
}

// logMember writes a debug line when a finding joins or leaves an incident.
// It is off by default; -v=4 shows which member keeps an incident open.
func logMember(change string, p *Incident, key detection.Key) {
	klog.V(4).InfoS("incident: finding "+change+" incident",
		"component", "incident", "incident", p.ID, "root", p.Root.String(),
		"entity", key.Entity.String(), "reason", key.Reason,
		"members", len(p.Members), "state", p.State.String())
}

// leavesAsHistory takes an eviction that is over out of its incident and
// reports whether it did.
func (m *Manager) leavesAsHistory(
	now time.Time, s detection.Finding, where placement,
) bool {
	if !m.outlivedEviction(s, where) {
		return false
	}
	m.leaveHistory(now, s)
	return true
}

// reroute sends a failure back to the incident whose story it belongs to:
// one that took over from its root, one it moved away from, the node
// outage it was suspected of, the node outage whose place it names, or
// the outage its node joined.
func (m *Manager) reroute(
	now time.Time, s detection.Finding, where placement,
) placement {
	where = m.retakeSuperseded(now, s, where)
	where = m.retakeFormer(now, s, where)
	where = m.retakeSuspect(now, s, where)
	where = m.retakeZone(now, where)
	return m.joinNodeGroup(s, where)
}

// takeCause adopts the cause of a placement. A grouped incident speaks for
// several nodes and keeps none.
func (p *Incident) takeCause(where placement) {
	switch {
	case p.Grouped():
	case where.cause != nil:
		p.Cause, p.CauseUnclear = where.cause, false
	case p.Cause == nil && where.unclear:
		p.CauseUnclear = true
	}
}
