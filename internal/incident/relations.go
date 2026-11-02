package incident

import (
	"cmp"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Two open incidents are related when the reasoning already ties one to
// the other: the failure chain of one goes on past its limit into the
// other's failures (rootcause.CauseRecord.Beyond, or a hop of the chain
// whose failure stayed with an incident announced earlier), or one is
// blamed on an object that is the root of the other (a node, a zone or
// pool merged into it). Nothing here decides an alert: messages and the
// dashboard read the lists, no fingerprint, tier or timer does.

// Related is another live incident an incident is caused by, or causes.
type Related struct {
	ID string
	// Title is the headline of the last message about it, else its
	// root.
	Title string
	Tier  Tier
	State State
	// Told is set when people heard about it: it was announced, its
	// announcement was in scope and a message about it was written.
	Told bool
}

// maxRelated bounds each list of related incidents.
const maxRelated = 5

// causes reports whether a's failure explains b's: both are live, and
// a's chain reaches b's root or one of its failures, or b's cause is an
// object a is rooted at.
func causes(a, b *Incident) bool {
	if a == b || a.State == Resolved || b.State == Resolved ||
		a.Root == b.Root {
		return false
	}
	if b.Cause != nil && b.Cause.Root != b.Root &&
		a.rootedAt(b.Cause.Root) {
		return true
	}
	if a.Cause == nil {
		return false
	}
	for _, hop := range slices.Concat(a.Cause.Hops, a.Cause.Beyond) {
		if !a.rootedAt(hop.Entity) && b.fails(hop.Entity) {
			return true
		}
	}
	return false
}

// rootedAt reports whether id is the incident's root, or the root of a
// node, zone or pool failure merged into it.
func (inc *Incident) rootedAt(id inventory.EntityID) bool {
	return id == inc.Root || slices.Contains(inc.Merged, id)
}

// fails reports whether id is the incident's root or the object of one
// of its failures.
func (inc *Incident) fails(id inventory.EntityID) bool {
	if id == inc.Root {
		return true
	}
	for key, f := range inc.Members {
		if key.Entity == id && !f.Advisory {
			return true
		}
	}
	return false
}

// relate fills the related lists of snap, a copy of a live incident,
// from the incidents the manager holds now. A resolved copy has none.
func (m *Manager) relate(snap *Incident) {
	m.relateAmong(snap, m.indexLive())
}

// relIndex lists the live incidents once, in ID order, and finds by
// entity the few that can be related to another, so relating every
// incident costs about one pass instead of every pair.
type relIndex struct {
	live []*Incident
	pos  map[*Incident]int
	// rooted: incidents rooted at (or merged from) the entity.
	rooted map[inventory.EntityID][]*Incident
	// failing: incidents whose root or failure is the entity.
	failing map[inventory.EntityID][]*Incident
	// hopped: incidents whose chain passes the entity.
	hopped map[inventory.EntityID][]*Incident
	// blamed: incidents whose cause root is the entity.
	blamed map[inventory.EntityID][]*Incident
}

// indexLive builds the relIndex of the live incidents.
func (m *Manager) indexLive() *relIndex {
	ix := &relIndex{
		pos:     map[*Incident]int{},
		rooted:  map[inventory.EntityID][]*Incident{},
		failing: map[inventory.EntityID][]*Incident{},
		hopped:  map[inventory.EntityID][]*Incident{},
		blamed:  map[inventory.EntityID][]*Incident{},
	}
	for i, id := range m.liveIDs() {
		q := m.incidents[id]
		ix.live = append(ix.live, q)
		ix.pos[q] = i
		for _, e := range append([]inventory.EntityID{q.Root}, q.Merged...) {
			ix.rooted[e] = append(ix.rooted[e], q)
		}
		for _, e := range q.failingEntities() {
			ix.failing[e] = append(ix.failing[e], q)
		}
		if q.Cause != nil {
			ix.blamed[q.Cause.Root] = append(ix.blamed[q.Cause.Root], q)
			for _, hop := range q.hops() {
				ix.hopped[hop.Entity] = append(ix.hopped[hop.Entity], q)
			}
		}
	}
	return ix
}

// failingEntities is the incident's root and the objects of its
// failures (see fails).
func (inc *Incident) failingEntities() []inventory.EntityID {
	out := []inventory.EntityID{inc.Root}
	for key, f := range inc.Members {
		if !f.Advisory {
			out = append(out, key.Entity)
		}
	}
	return out
}

// hops is the chain of the incident's cause, within and beyond its
// limit.
func (inc *Incident) hops() []rootcause.Hop {
	return slices.Concat(inc.Cause.Hops, inc.Cause.Beyond)
}

// candidates are the incidents that can be related to p, in live
// order: a superset of the ones causes accepts, from the entity indexes.
func (ix *relIndex) candidates(p *Incident) []*Incident {
	seen := map[*Incident]bool{}
	var out []*Incident
	add := func(list []*Incident) {
		for _, q := range list {
			if !seen[q] {
				seen[q] = true
				out = append(out, q)
			}
		}
	}
	if p.Cause != nil {
		add(ix.rooted[p.Cause.Root])
		for _, hop := range p.hops() {
			add(ix.failing[hop.Entity])
		}
	}
	for _, e := range append([]inventory.EntityID{p.Root}, p.Merged...) {
		add(ix.blamed[e])
	}
	for _, e := range p.failingEntities() {
		add(ix.hopped[e])
	}
	slices.SortFunc(out, func(a, b *Incident) int {
		return cmp.Compare(ix.pos[a], ix.pos[b])
	})
	return out
}

// relateAmong is relate with the live incidents already indexed, so a
// caller copying every incident indexes them once.
func (m *Manager) relateAmong(snap *Incident, ix *relIndex) {
	snap.CausedBy, snap.Causing = nil, nil
	p := m.incidents[snap.ID]
	if p == nil || p.State == Resolved {
		return
	}
	for _, q := range ix.candidates(p) {
		if len(snap.CausedBy) < maxRelated && causes(q, p) {
			snap.CausedBy = append(snap.CausedBy, relatedOf(q))
		} else if len(snap.Causing) < maxRelated && causes(p, q) {
			snap.Causing = append(snap.Causing, relatedOf(q))
		}
	}
}

// relatedOf is q as a related incident.
func relatedOf(q *Incident) Related {
	title := strings.TrimSpace(q.Headline)
	if title == "" {
		title = describe(q.Root)
	}
	return Related{ID: q.ID, Title: title, Tier: q.Tier, State: q.State,
		Told: q.Headline != "" && wasAnnounced(q) && q.Scope != ScopeOut}
}

// CauseTold is the incident this one is caused by that people already
// heard about, the loudest first, or false when there is none.
func (inc Incident) CauseTold() (Related, bool) {
	best, found := Related{}, false
	for _, r := range inc.CausedBy {
		if r.Told && (!found || r.Tier > best.Tier) {
			best, found = r, true
		}
	}
	return best, found
}
