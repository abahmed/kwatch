package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// PlaneWindow is how close the openings of two control-plane failures
// must be to be one outage: they began within one settle window.
const PlaneWindow = 2 * time.Minute

// planeDepth orders what the control plane is built on: etcd stores
// what the API server serves, and what the scheduler, the controller
// manager and the kubelets' status updates go through the API server
// for. A failure higher up that opens beside one lower down is that
// one's outage seen from above, not a second problem.
func planeDepth(id inventory.EntityID) (int, bool) {
	switch id.Kind {
	case kube.Etcd.Kind:
		return 0, true
	case kube.APIServer.Kind:
		return 1, true
	case kube.Scheduler.Kind, kube.ControllerManager.Kind:
		return 2, true
	}
	return 0, false
}

// joinPlane places a control-plane failure in the incident of the
// failure it rests on when both opened within PlaneWindow: the
// component below it, or, for the API server, nodes that stopped
// reporting. One that already joined stays.
func (m *Manager) joinPlane(
	now time.Time, s detection.Finding, where placement,
) placement {
	depth, ok := planeDepth(where.root)
	if !ok || s.Advisory {
		return where
	}
	for _, id := range m.liveIDs() {
		q := m.incidents[id]
		if q.State != Settling && q.State != Open || q.Root == where.root {
			continue
		}
		if q.fails(where.root) || m.restsOn(now, q, where.root, depth) {
			return placement{root: q.Root}
		}
	}
	return where
}

// restsOn reports that the failure of root, at depth, is explained by
// q: a control-plane component lower down, or a node outage for the API
// server, opened within PlaneWindow of now.
func (m *Manager) restsOn(
	now time.Time, q *Incident, root inventory.EntityID, depth int,
) bool {
	if now.Sub(q.Opened).Abs() > PlaneWindow {
		return false
	}
	if below, ok := planeDepth(q.Root); ok {
		return below < depth
	}
	return depth == 1 && nodeFamily(q.Root.Kind) && nodePaging(q)
}
