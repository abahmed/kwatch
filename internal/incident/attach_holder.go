package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// holderKinds are the workloads a live incident can already speak for.
var holderKinds = map[inventory.Kind]bool{
	kube.KindDeployment: true, kube.KindStatefulSet: true,
	kube.KindDaemonSet: true, kube.KindJob: true,
}

// joinHolder places a failure that arrives late in the live incident
// that already speaks for its workload, in two cases. A disruption
// budget that holds a drain because its workload has too few healthy
// pods is the workload's story: it joins the workload's incident. And a
// workload that an outage of a node it runs on already names as
// affected, while that node still fails, joins the node's incident when
// it fails some other way (a volume the lost node still holds): the
// node is why, and a second incident would repeat the story.
func (m *Manager) joinHolder(
	s detection.Finding, where placement,
) placement {
	if m.model == nil || s.Advisory || where.root.Name == "" {
		return where
	}
	w := where.root
	if where.root.Kind == kube.KindPDB {
		w = workloadOf(m.model, where.root)
	} else if where.cause != nil && where.cause.Rule != selfRow {
		return where
	}
	if !holderKinds[w.Kind] {
		return where
	}
	p := m.workloadIncident(w)
	if p == nil || p.Root == where.root ||
		p.State != Settling && p.State != Open ||
		p.Cause != nil && p.Cause.Root == where.root {
		return where
	}
	if w == where.root && !(nodeFamily(p.Root.Kind) && nodePaging(p)) {
		return where
	}
	return placement{root: p.Root}
}

// joinOutages places a failure in the outage it belongs to: the nodes
// lost together, the incident that already speaks for its workload and
// the control-plane failure it rests on.
func (m *Manager) joinOutages(
	now time.Time, s detection.Finding, where placement,
) placement {
	where = m.joinNodeGroup(now, s, where)
	where = m.joinHolder(s, where)
	return m.joinPlane(now, s, where)
}
