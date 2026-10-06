package incident

import (
	"slices"
	"strconv"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// selfRow names the explain row of a workload blamed for itself.
const selfRow = "self"

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

// areaPlacer places the failures of one area. It indexes which cause
// covers each failure and remembers each cause's record.
type areaPlacer struct {
	s    explain.Snapshot
	area explain.Area
	// covering maps a failure to the index of the first cause that
	// covers it.
	covering map[inventory.EntityID]int
	records  map[int]rootcause.CauseRecord
	blamed   map[int]bool
	// outside caches, per own root, whether a rejected candidate lies
	// outside that workload.
	outside map[inventory.EntityID]bool
}

func newAreaPlacer(s explain.Snapshot, area explain.Area) *areaPlacer {
	covering := map[inventory.EntityID]int{}
	for i, cause := range area.Causes {
		for _, covered := range cause.Covers {
			if _, seen := covering[covered]; !seen {
				covering[covered] = i
			}
		}
	}
	return &areaPlacer{s: s, area: area, covering: covering,
		records: map[int]rootcause.CauseRecord{}, blamed: map[int]bool{},
		outside: map[inventory.EntityID]bool{}}
}

// placementOf finds the incident a failure of the area belongs to. A
// failure no stated cause covers belongs to its own workload, without
// a cause. So does a failure whose cause stays inside its own workload
// (a Deployment explained by its crashing pods), unless a change of
// that workload is blamed or the row names a fault of the workload
// itself (a memory limit, a failing sidecar: explain.InsideRow), and a
// failure blamed on itself by the self row: "it broke on its own" names
// no cause, whatever changed. A failure left without a cause while
// candidates outside its workload were dropped has an unclear cause.
func (a *areaPlacer) placementOf(failure inventory.EntityID) placement {
	own := defaultRoot(a.s.Model, failure)
	out := placement{root: own, unverified: a.area.Unverified,
		checked:    checkedKinds(a.area.Trace.Checked[failure]),
		considered: consideredCauses(a.area.Alternatives)}
	i, ok := a.covering[failure]
	if !ok {
		out.unclear = a.rejectedOutside(own)
		return out
	}
	cause := a.area.Causes[i]
	root := defaultRoot(a.s.Model, cause.Root)
	if root == own && (cause.Row == selfRow || (!a.blamesChange(i) &&
		!explain.InsideRow(cause.Row))) {
		out.unclear = a.rejectedOutside(own)
		return out
	}
	record := a.record(i)
	out.root, out.cause = root, &record
	return out
}

// maxConsidered bounds the alternatives an incident remembers.
const maxConsidered = 3

// consideredCauses words the alternatives the solver weighed and did
// not choose, best first, for the audit log: "node//n1 (node-not-ready,
// 0.42)". They let a reader see what the engine ruled out.
func consideredCauses(alternatives []explain.Cause) []string {
	var out []string
	for i, c := range alternatives {
		if i == maxConsidered {
			break
		}
		out = append(out, c.Root.String()+" ("+c.Row+", "+
			strconv.FormatFloat(c.Confidence, 'f', 2, 64)+")")
	}
	return out
}

// checkedKinds names the kinds of the checked entities, sorted and
// without repeats.
func checkedKinds(ids []inventory.EntityID) []string {
	var out []string
	for _, id := range ids {
		out = mergeSorted(out, string(id.Kind))
	}
	return out
}

// mergeSorted inserts value into the sorted list unless it is there.
func mergeSorted(sorted []string, value string) []string {
	i, found := slices.BinarySearch(sorted, value)
	if found {
		return sorted
	}
	return slices.Insert(sorted, i, value)
}

// rejectedOutside reports whether the area dropped a candidate that is
// not part of own's workload, computed once per own root. A rejection
// of the workload itself or of its pods says nothing about an upstream
// cause.
func (a *areaPlacer) rejectedOutside(own inventory.EntityID) bool {
	outside, ok := a.outside[own]
	if ok {
		return outside
	}
	for _, r := range a.area.Trace.Rejected {
		if !r.Insufficient && defaultRoot(a.s.Model, r.Root) != own {
			outside = true
			break
		}
	}
	a.outside[own] = outside
	return outside
}

// blamesChange reports whether cause i blames a change, computed once.
func (a *areaPlacer) blamesChange(i int) bool {
	blamed, ok := a.blamed[i]
	if !ok {
		blamed = len(a.s.BlamedChanges(a.area.Causes[i])) > 0
		a.blamed[i] = blamed
	}
	return blamed
}

// record is the incident record of cause i, built once. Each caller
// gets its own copy.
func (a *areaPlacer) record(i int) rootcause.CauseRecord {
	record, ok := a.records[i]
	if !ok {
		record = a.s.Record(a.area.Causes[i])
		record.Unverified = a.area.Unverified
		a.annotate(&record, a.area.Causes[i])
		a.records[i] = record
	}
	return record
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
	key := s.Key()
	revised, rerooted := false, false
	if previous, ok := m.byMember[key]; ok && !m.holds(previous, where.root) {
		if m.keepsMember(previous, s, where) {
			return
		}
		revised = true
		rerooted = m.revise(now, s, previous, where.root)
	}
	p := m.incident(now, where.root, s.Mode)
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
	if where.cause != nil {
		p.Cause, p.CauseUnclear = where.cause, false
	} else if p.Cause == nil && where.unclear {
		p.CauseUnclear = true
	}
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
	previous string, s detection.Finding, where placement,
) bool {
	key := s.Key()
	if !m.beganTooLate(previous, where.cause) &&
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
	if p.Announced.IsZero() || computed >= p.Tier {
		return computed
	}
	return p.Tier
}

// refresh recomputes what depends on the whole member set of every
// incident that changed in this Apply: mode, impact and tier. Doing it
// once per incident keeps a storm of members linear.
func (m *Manager) refresh(model inventory.Reader) {
	for id := range m.changed {
		if p := m.incidents[id]; p != nil {
			p.Mode = incidentMode(p)
			p.Impact = impact(model, p)
			p.trafficLost = trafficLost(model, p)
			p.servingDown = servingDown(model, p)
			p.admissionBlocked = admissionBlocked(model, p)
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
