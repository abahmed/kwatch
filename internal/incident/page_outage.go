package incident

import "time"

// Node outages are paged once. Nodes that fail in the same minute are
// joined into one incident (node_group.go) when the solver names them
// alike; when it names the same outage by a node for one look and by a
// zone for the next, two incidents settle a few seconds apart. The
// second one is told to chat and holds its page: the first page already
// says the nodes are down.

// outageSibling is another node outage that already paged and opened
// within one settle of p, or nil.
func (m *Manager) outageSibling(p *Incident) *Incident {
	if !nodeFamily(p.Root.Kind) {
		return nil
	}
	for _, id := range m.sortedIDs() {
		o := m.incidents[id]
		if o != p && nodeFamily(o.Root.Kind) && o.State != Resolved &&
			wasAnnounced(o) && o.Tier == Page &&
			absGap(p.Opened, o.Opened) <= m.cfg.Settle {
			return o
		}
	}
	return nil
}

// absGap is the distance between two times.
func absGap(a, b time.Time) time.Duration {
	if d := a.Sub(b); d >= 0 {
		return d
	}
	return b.Sub(a)
}

// holdSiblingPage holds the page of a node outage whose sibling paged.
func (m *Manager) holdSiblingPage(p *Incident, now time.Time) {
	if p.Tier != Page || p.Delivery.PageHeld() {
		return
	}
	if o := m.outageSibling(p); o != nil {
		p.Delivery.HoldAtNotify()
		p.Tier = Notify
		p.note(now, "same outage as "+describe(o.Root)+", already paged")
	}
}
