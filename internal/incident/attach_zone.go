package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A node outage that spreads slowly names itself twice: first by the
// node that went down, minutes later by the zone or pool that now fails
// as a whole. Told twice it would page twice for one outage, and the
// first alert would be closed as "moved" while the outage goes on. The
// incident that already speaks for the nodes of the place takes the
// place as its root instead: one thread, one alert, one revised cause.

// retakeZone gives the first failure placed at a zone or pool that no
// incident speaks for to the announced incident of one of its nodes, by
// moving that incident to the place. A place that already has an incident
// keeps it: the nodes join there through the usual revision.
func (m *Manager) retakeZone(
	now time.Time, where placement,
) placement {
	place := where.root
	if place.Kind != kube.KindZone && place.Kind != kube.KindNodePool ||
		m.holdsLive(place) {
		return where
	}
	if q := m.announcedNodeIn(place); q != nil {
		m.dropCoveredMerges(q, place)
		m.reroot(now, q, place)
	}
	return where
}

// announcedNodeIn is the announced live incident, the lowest ID first,
// that speaks for a node of place, or nil.
func (m *Manager) announcedNodeIn(place inventory.EntityID) *Incident {
	for _, id := range m.sortedIDs() {
		q := m.incidents[id]
		if q.State != Resolved && wasAnnounced(q) &&
			q.Root.Kind == kube.KindNode && m.placeOf(q.Root, place) {
			return q
		}
	}
	return nil
}

// dropCoveredMerges takes the nodes of place out of the outage q
// speaks for: the place now stands for them (see coversGroup).
func (m *Manager) dropCoveredMerges(q *Incident, place inventory.EntityID) {
	kept := q.Merged[:0:0]
	for _, root := range q.Merged {
		if m.placeOf(root, place) {
			delete(m.groups, root.String())
			continue
		}
		kept = append(kept, root)
	}
	q.Merged = kept
}
