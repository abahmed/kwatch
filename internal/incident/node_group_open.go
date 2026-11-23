package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// NodeGroupWindow is how soon after a node of an open outage was lost
// another node lost the same way still belongs to it. Nodes that go
// down one after another, minutes apart, share a cause that the first
// page already told about: one incident, not a page for each.
const NodeGroupWindow = 5 * time.Minute

// openOutage is the live node outage that already went out and that s,
// a node lost the same way within NodeGroupWindow of its last lost
// node, belongs to; nil when none. A finding that is no node loss (a
// pod on the node) follows its node's outage and joins any open one.
func (m *Manager) openOutage(now time.Time, s detection.Finding) *Incident {
	var found *Incident
	for _, id := range m.liveIDs() {
		p := m.incidents[id]
		if p.State != Open || !nodeFamily(p.Root.Kind) ||
			!nodePaging(p) && !p.Grouped() || m.takenOver(p) {
			continue
		}
		if !lostSimilarly(now, p, s) {
			continue
		}
		if found == nil || p.Opened.Before(found.Opened) {
			found = p
		}
	}
	return found
}

// lostSimilarly reports whether p lost a node with the reason of s, and
// the last one within NodeGroupWindow of when s began.
func lostSimilarly(now time.Time, p *Incident, s detection.Finding) bool {
	at := s.Since
	if at.IsZero() || at.After(now) {
		at = now
	}
	for _, f := range p.Members {
		if !nodeLostFinding(f) ||
			nodeLostFinding(s) && f.Reason != s.Reason {
			continue
		}
		last := f.Since
		if last.IsZero() {
			last = p.Opened
		}
		if at.Sub(last).Abs() <= NodeGroupWindow {
			return true
		}
	}
	return false
}
