package incident

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A deleted Node cannot fail any more. Most of its findings clear when
// the node goes, but a finding read from what outlives the node (its
// recent events, a restored member) is never cleared by a detector,
// and would keep the incident open for ever. So every finding about a
// node that was deleted leaves its incident, whatever its reason, and
// none joins one again: the incident recovers and resolves, and its
// resolve says the node was removed (noteRemoved).

// nodeRemoved reports a Node that was in the model and was deleted.
// A node kwatch never saw (a finding raised before the node was
// listed) is not claimed removed, nor one it cannot observe now.
func (m *Manager) nodeRemoved(id inventory.EntityID) bool {
	return id.Kind == kube.KindNode && m.model != nil &&
		!m.model.Exists(id) && removedSince(m.model, id, time.Time{}) &&
		m.rootVerifiable(id)
}

// dropRemovedNodes takes every finding about a removed node out of its
// incident, as if it had cleared.
func (m *Manager) dropRemovedNodes(now time.Time) {
	var gone []detection.Finding
	for key, id := range m.byMember {
		if !m.nodeRemoved(key.Entity) {
			continue
		}
		if p := m.incidents[id]; p != nil {
			gone = append(gone, p.Members[key])
		}
	}
	sort.Slice(gone, func(i, j int) bool {
		a, b := gone[i].Key(), gone[j].Key()
		return a.Entity.String()+a.Reason < b.Entity.String()+b.Reason
	})
	for _, s := range gone {
		m.detach(now, s, false)
	}
	if len(gone) > 0 {
		m.refresh(m.model, now)
	}
}
