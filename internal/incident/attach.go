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

// placement is where the findings of one failure belong: the root of
// their incident, the cause that explains them and what kwatch could
// not check on the way.
type placement struct {
	root       inventory.EntityID
	cause      *rootcause.CauseRecord
	unverified []string
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
	out := placement{root: own, unverified: a.area.Unverified}
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
		if defaultRoot(a.s.Model, r.Root) != own {
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
		a.records[i] = record
	}
	return record
}

// attach makes s a member of the incident of where.root. A finding that
// belonged to another incident is moved: its cause was revised.
func (m *Manager) attach(
	now time.Time, s detection.Finding, where placement,
) {
	key := s.Key()
	revised, rerooted := false, false
	if previous, ok := m.byMember[key]; ok && !m.holds(previous, where.root) {
		revised = true
		rerooted = m.revise(now, s, previous, where.root)
	}
	p := m.incident(now, where.root)
	if revised && !rerooted {
		p.note(now, "cause revised: now explained by "+describe(where.root))
	}
	if _, known := p.Members[key]; !known {
		p.note(now, s.Summary+" ("+describe(s.Entity)+")")
	}
	p.Members[key] = s
	logMember("added to", p, key)
	p.SupersededBy = ""
	if where.cause != nil {
		p.Cause, p.CauseUnclear = where.cause, false
	} else if p.Cause == nil && where.unclear {
		p.CauseUnclear = true
	}
	p.Unverified = rootcause.MergeUnverified(p.Unverified, where.unverified)
	m.byMember[key] = p.ID
	m.changed[p.ID] = true
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
			p.admissionBlocked = admissionBlocked(model, p)
			p.routedMissing = routedMissing(model, p)
			p.impactPeak = max(p.impactPeak, impactSize(p))
			p.Tier = m.override.apply(p, tier(p))
		}
	}
	clear(m.changed)
}

// holds reports whether incident id is the current incident of root.
func (m *Manager) holds(id string, root inventory.EntityID) bool {
	p := m.lookup(root)
	return p != nil && p.ID == id
}

// revise moves s away from incident previous after its cause changed to
// root. When that leaves an announced incident empty, the incident is
// either superseded, because root already has its own announced
// incident, or moved to root so the conversation continues under the
// same ID. It reports whether the incident was moved.
func (m *Manager) revise(
	now time.Time, s detection.Finding, previous string,
	root inventory.EntityID,
) bool {
	key := s.Key()
	delete(m.byMember, key)
	old := m.incidents[previous]
	if old == nil {
		return false
	}
	delete(old.Members, key)
	logMember("moved out of", old, key)
	m.changed[old.ID] = true
	old.note(now, "cause revised: "+describe(s.Entity)+
		" is now explained by "+describe(root))
	if len(old.Members) > 0 || !wasAnnounced(old) {
		return false
	}
	if target := m.lookup(root); target != nil && wasAnnounced(target) {
		old.SupersededBy = target.ID
		return false
	}
	m.reroot(now, old, root)
	return true
}

// reroot moves incident p to root and marks its next update as a cause
// revision. An incident of root that is still settling is merged into p:
// nobody has heard of it yet.
func (m *Manager) reroot(
	now time.Time, p *Incident, root inventory.EntityID,
) {
	// The old root's cause does not explain the new root: the members
	// that move here bring the new one.
	p.Cause, p.CauseUnclear = nil, false
	if target := m.lookup(root); target != nil && target.State == Settling {
		for key, s := range target.Members {
			p.Members[key] = s
			m.byMember[key] = p.ID
		}
		p.Cause, p.CauseUnclear = target.Cause, target.CauseUnclear
		p.Unverified = rootcause.MergeUnverified(
			p.Unverified, target.Unverified)
		delete(m.incidents, target.ID)
	}
	m.unindex(p)
	p.Root = root
	m.index(p)
	p.revised, p.revisedAt = true, now
	p.note(now, "cause revised: now explained by "+describe(root))
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
	if p := m.incidents[id]; p != nil {
		delete(p.Members, key)
		logMember("removed from", p, key)
		p.note(now, "recovered: "+s.Summary+" ("+describe(s.Entity)+")")
	}
}

// incident returns the live incident for root, opening a new one when
// there is none.
func (m *Manager) incident(
	now time.Time, root inventory.EntityID,
) *Incident {
	previous := m.lookup(root)
	if previous != nil && previous.State != Resolved {
		return previous
	}
	p := &Incident{
		ID: m.newID(now), Root: root, State: Settling, Opened: now,
		Members:     make(map[detection.Key]detection.Finding),
		Occurrences: []time.Time{now},
	}
	if previous != nil {
		m.recur(p, previous, now)
	}
	m.incidents[p.ID] = p
	m.index(p)
	m.noteOpened(p.ID)
	return p
}

// recurrenceWeek is the window of "the third time this week": the
// timeline counts the occurrences inside it.
const recurrenceWeek = 7 * 24 * time.Hour

// recur links a new incident to the resolved incident of the same root
// and carries its history over, so flapping and daily routines are still
// recognised. A recurrence within FlapWindow of the resolve counts as a
// cycle, so an incident that stays healthy longer than its hold between
// failures still doubles its hold and flaps.
func (m *Manager) recur(p, previous *Incident, now time.Time) {
	p.Previous = previous.ID
	p.Cycles = recent(previous.Cycles, now, m.cfg.FlapWindow)
	if now.Sub(previous.Resolved) <= m.cfg.FlapWindow {
		p.Cycles = append(p.Cycles, now)
	}
	p.Occurrences = appendOccurrence(previous.Occurrences, now)
	if previous.SupersededBy == "" {
		p.History = appendHistory(previous.History, occurrenceOf(previous))
	}
	week := recent(p.Occurrences, now, recurrenceWeek)
	p.note(now, "happened again ("+ordinal(len(week))+
		" time this week, last time "+previous.ID+")")
	// The recurrence carries everything the predecessor knew, so keeping
	// the predecessor would only grow memory with every blip.
	m.forget(previous)
}

// logMember writes a debug line when a finding joins or leaves an incident.
// It is off by default; -v=4 shows which member keeps an incident open.
func logMember(change string, p *Incident, key detection.Key) {
	klog.V(4).InfoS("incident: finding "+change+" incident",
		"component", "incident", "incident", p.ID, "root", p.Root.String(),
		"entity", key.Entity.String(), "reason", key.Reason,
		"members", len(p.Members), "state", p.State.String())
}
