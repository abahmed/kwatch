package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The restore grace exists because findings come back after a restart: a
// restored incident with no members may just be waiting for them. A node
// whose own Ready condition says it is ready again has nothing to wait
// for, but only when Ready is all the incident was about: a node that is
// Ready can still be under disk or memory pressure, so an incident that
// ever had another reason of its own keeps the grace. A node that
// recovered while kwatch was down does not wait for the grace and the hold
// to say so, and is told as it was: failing until the node came back, not
// until the grace ended.

// readyEndsIt reports an incident whose root reasons are all about the
// node not being Ready (NodeNotReady, NodeHeartbeatStale): the Ready
// condition is the recovery of those and of nothing else. With no reason
// recorded (a record from before they were kept) it is not known, and the
// grace applies.
func readyEndsIt(p *Incident) bool {
	if len(p.rootReasons) == 0 {
		return false
	}
	for reason := range p.rootReasons {
		if reason != reasons.NodeNotReady &&
			reason != reasons.NodeHeartbeatStale {
			return false
		}
	}
	return true
}

// recoveredWhileDown reports a restored incident rooted at a node that the
// synced model shows ready, with no member back, when Ready is the
// recovery of everything the incident was about (readyEndsIt), and when
// the node became ready (zero when the model does not say).
func (m *Manager) recoveredWhileDown(p *Incident) (time.Time, bool) {
	if !p.restored || len(p.Members) > 0 || p.Root.Kind != kube.KindNode ||
		!readyEndsIt(p) || m.model == nil ||
		(m.cfg.Verifiable != nil && !m.cfg.Verifiable(kube.KindNode)) {
		return time.Time{}, false
	}
	ready, ok := m.model.Attribute(p.Root, kube.AttrReady)
	if !ok {
		return time.Time{}, false
	}
	if up, isBool := ready.Value.AsBool(); !isBool || !up {
		return time.Time{}, false
	}
	since, ok := m.model.Attribute(p.Root, kube.AttrReadySince)
	if !ok {
		return time.Time{}, true
	}
	return since.Value.AsTime(), true
}

// recoveryStart is when the failure of p ended, as of now: the moment a
// node that recovered while kwatch was down became ready again, when the
// model says so, otherwise now.
func (m *Manager) recoveryStart(p *Incident, now time.Time) time.Time {
	if since, ok := m.recoveredWhileDown(p); ok && since.After(p.Opened) &&
		since.Before(now) {
		return since
	}
	return now
}
