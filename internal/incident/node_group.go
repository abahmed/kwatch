package incident

import (
	"slices"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Node outages. Nodes, zones and pools that fail in the same settle
// window are one outage, so they share one incident and one page. The
// first failure opens the incident; the next ones join it while it is
// still settling. The solver may name the failing place differently from
// one look to the next (a zone, then its nodes), so a joined root keeps
// pointing at the incident, and its findings stay in it.

// nodeFamily reports a kind of place a node outage is rooted at.
func nodeFamily(kind inventory.Kind) bool {
	return kind == kube.KindNode || kind == kube.KindZone ||
		kind == kube.KindNodePool
}

// nodesFailing reports a cause that blames failing nodes: one that is
// not ready, or a zone or pool with several of them.
func nodesFailing(c *rootcause.CauseRecord) bool {
	if c == nil || !nodeFamily(c.Root.Kind) {
		return false
	}
	switch c.Rule {
	case explain.RowNodeNotReady, explain.RowZoneFailing,
		explain.RowNodePoolFailing:
		return true
	}
	return false
}

// nodeLostFinding reports a finding that says a node is not ready or has
// stopped reporting.
func nodeLostFinding(s detection.Finding) bool {
	return s.Entity.Kind == kube.KindNode && !s.Advisory &&
		(s.Reason == reasons.NodeNotReady ||
			s.Reason == reasons.NodeHeartbeatStale)
}

// Grouped reports an incident that several node, zone or pool failures
// of one settle window share.
func (p Incident) Grouped() bool { return len(p.Merged) > 0 }

// groupOf is the live incident root has joined, or nil.
func (m *Manager) groupOf(root inventory.EntityID) *Incident {
	id, ok := m.groups[root.String()]
	if !ok {
		return nil
	}
	if p := m.incidents[id]; p != nil && p.State != Resolved {
		return p
	}
	delete(m.groups, root.String())
	return nil
}

// settlingOutage is the earliest incident of a node outage that is still
// settling, or nil.
func (m *Manager) settlingOutage() *Incident {
	var first *Incident
	for _, p := range m.incidents {
		if p.State != Settling || !nodeFamily(p.Root.Kind) ||
			!(nodesFailing(p.Cause) || p.Grouped() || nodePaging(p)) ||
			m.takenOver(p) {
			continue
		}
		if first == nil || p.Opened.Before(first.Opened) ||
			(p.Opened.Equal(first.Opened) && p.ID < first.ID) {
			first = p
		}
	}
	return first
}

// takenOver reports a settling node incident whose node a settling zone
// or pool incident now speaks for: its failures are about to move there.
func (m *Manager) takenOver(p *Incident) bool {
	for _, q := range m.incidents {
		if q != p && q.State == Settling && q.Root.Kind != kube.KindNode &&
			m.placeOf(p.Root, q.Root) {
			return true
		}
	}
	return false
}

// joinNodeGroup places a node, zone or pool failure in the incident of
// the outage it belongs to: one it already joined, or one that is still
// settling when nothing holds the root yet.
func (m *Manager) joinNodeGroup(
	s detection.Finding, where placement,
) placement {
	root := where.root
	if !nodeFamily(root.Kind) {
		return where
	}
	g := m.groupOf(root)
	if g == nil && (m.holdsLive(root) ||
		!(nodesFailing(where.cause) || nodeLostFinding(s))) {
		return where
	}
	if g == nil {
		g = m.settlingOutage()
		if g == nil || g.Root == root || m.placeOf(root, g.Root) ||
			m.coversGroup(root, g) {
			return where
		}
		m.merge(g, root)
	}
	return placeIn(g, where)
}

// placeOf reports a node that is part of the zone or pool place: the
// place's own incident already speaks for it.
func (m *Manager) placeOf(node, place inventory.EntityID) bool {
	if m.model == nil || node.Kind != kube.KindNode {
		return false
	}
	return slices.Contains(
		m.model.Related(node, inventory.PartOf, inventory.Outgoing), place)
}

// holdsLive reports a live incident of root.
func (m *Manager) holdsLive(root inventory.EntityID) bool {
	p := m.lookup(root)
	return p != nil && p.State != Resolved
}

// merge makes root part of g's outage. The incident no longer has one
// cause: it speaks for every node that is down.
func (m *Manager) merge(g *Incident, root inventory.EntityID) {
	g.Merged = append(g.Merged, root)
	g.Cause, g.CauseUnclear = nil, false
	m.groups[root.String()] = g.ID
	m.changed[g.ID] = true
}

// placeIn places a failure in g, which has no cause of its own to
// overwrite.
func placeIn(g *Incident, where placement) placement {
	where.root, where.cause, where.unclear = g.Root, nil, false
	return where
}

// staysInGroup reports a finding of a grouped incident that the solver
// now places at another node, zone or pool: the same outage, named
// differently. It stays.
func (m *Manager) staysInGroup(previous string, where placement) bool {
	p := m.incidents[previous]
	return p != nil && p.Grouped() && nodeFamily(where.root.Kind) &&
		!m.coversGroup(where.root, p)
}

// coversGroup reports a zone or pool that holds the nodes of g's outage:
// the place failing as a whole replaces the list of its nodes, as the
// zone's own incident always has.
func (m *Manager) coversGroup(place inventory.EntityID, g *Incident) bool {
	if place.Kind == kube.KindNode {
		return false
	}
	for _, root := range append([]inventory.EntityID{g.Root}, g.Merged...) {
		if m.placeOf(root, place) {
			return true
		}
	}
	return false
}

// restoreGroup rebuilds the roots a restored incident merged.
func (m *Manager) restoreGroup(p *Incident) {
	for _, root := range p.Merged {
		m.groups[root.String()] = p.ID
	}
}

// nodePaging reports an incident whose root node is lost seriously
// enough to page.
func nodePaging(p *Incident) bool {
	for key, s := range p.Members {
		if key.Entity == p.Root && nodeLostFinding(s) {
			return true
		}
	}
	return false
}
