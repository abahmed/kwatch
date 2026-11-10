package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// scaleSideModes are the failure modes nodes coming and going cause by
// themselves: pods waiting for a node or starting on a cold node (its
// image pull included, which is ModeCreating), pods evicted or lost
// with a removed node, and the workloads and Services short of ready
// pods meanwhile. A crash loop, an OOM kill, a bad configuration or a
// pull that failed is not among them: a failed pull is an error, not a
// wait.
var scaleSideModes = map[detection.Mode]bool{
	detection.ModePending:            true,
	detection.ModeUnschedulable:      true,
	detection.ModeCreating:           true,
	detection.ModeInitializing:       true,
	detection.ModeEvicted:            true,
	detection.ModeEvicting:           true,
	detection.ModeStatusUnknown:      true,
	detection.ModeStuckDeleting:      true,
	detection.ModeNotReady:           true,
	detection.ModeUnavailable:        true,
	detection.ModeNoEndpoints:        true,
	detection.ModeBackendsDegraded:   true,
	detection.ModeDraining:           true,
	detection.ModeKubeletUnreachable: true,
}

const (
	// scaleLinkSlack is how long before the wave a pod may have been
	// created and still count as moved by it: the drain that empties a
	// node comes before the node goes.
	scaleLinkSlack = 5 * time.Minute
	// scaleOnsetSlack is how long before the wave a failure may have
	// begun and still be its side effect. An older one is not.
	scaleOnsetSlack = 10 * time.Minute
	// scaleJoinGrace is how long a node that joined during the wave
	// may be not ready before that is a failure of its own.
	scaleJoinGrace = 5 * time.Minute
)

// scaleHeld reports a settling incident that waits, at now, for the
// cluster's scaling wave to end before its first message: every
// failure in it is a scaling side effect tied to the wave's nodes, and
// no change is blamed for it. A notification is held too: told now, it
// would only turn into a page a minute later, when the workload behind
// it has no ready pod left. Once held it stays held while it settles,
// so a tier that drops meanwhile does not announce it early; a failure
// that is no side effect joining it ends the hold.
func (m *Manager) scaleHeld(p *Incident, now time.Time) bool {
	end, ok := m.scaleHoldEnd(p)
	return ok && now.Before(end)
}

// scaleSettle is how long a held page settles: until its hold ends
// (scaleHoldEnd), and never less than settle.
func (m *Manager) scaleSettle(p *Incident, settle time.Duration) time.Duration {
	end, ok := m.scaleHoldEnd(p)
	if !ok {
		return settle
	}
	return max(settle, end.Sub(p.Opened))
}

// scaleHoldEnd is when the scaling hold of p ends; ok is false when p
// is not held. An incident opened inside the wave is judged by the
// wave. Any other keeps the end it was last given, while its failures
// are still side effects: after a restart the wave is not known any
// more, and the page must not go out because kwatch restarted.
func (m *Manager) scaleHoldEnd(p *Incident) (time.Time, bool) {
	if p.State != Settling {
		delete(m.scaleHolds, p.ID)
		return time.Time{}, false
	}
	end, ok := m.savedHoldEnd(p)
	if m.inWave(p) {
		if end, ok = m.waveHoldEnd(p); !ok {
			p.marks.ScaleHeldUntil = time.Time{}
		}
	}
	if !ok {
		delete(m.scaleHolds, p.ID)
		return time.Time{}, false
	}
	if !m.scaleHolds[p.ID] {
		if p.Tier < Notify {
			return time.Time{}, false
		}
		if m.scaleHolds == nil {
			m.scaleHolds = map[string]bool{}
		}
		m.scaleHolds[p.ID] = true
	}
	p.marks.ScaleHeldUntil = end
	return end, true
}

// inWave reports an incident opened inside the latest wave.
func (m *Manager) inWave(p *Incident) bool {
	end := m.wave.holdEnd()
	return !end.IsZero() && !p.Opened.Before(m.wave.started) &&
		p.Opened.Before(end)
}

// savedHoldEnd is the end the hold of p was last given
// (Marks.ScaleHeldUntil), honoured only while every failure of p is
// still a side-effect mode. A failure of another kind ends it for
// good; an incident whose failures are not back yet after a restart
// keeps it.
func (m *Manager) savedHoldEnd(p *Incident) (time.Time, bool) {
	until := p.marks.ScaleHeldUntil
	if until.IsZero() {
		return time.Time{}, false
	}
	if !sideModesOnly(p) {
		if hasFailingMember(p) {
			p.marks.ScaleHeldUntil = time.Time{}
		}
		return time.Time{}, false
	}
	return until, true
}

// waveHoldEnd judges an incident opened inside the wave: whether the
// wave explains it, and until when it is held. The hold ends with the
// wave's own, and no later than scaleMaxHold after the failure's onset
// (scaleMaxHoldServing when users lose traffic or nothing serves), nor
// later than scaleJoinGrace after a failing joined node was first seen.
func (m *Manager) waveHoldEnd(p *Incident) (time.Time, bool) {
	if m.model == nil || !sideModesOnly(p) || !m.waveLinked(p) {
		return time.Time{}, false
	}
	end, onset := m.wave.holdEnd(), p.Opened
	for _, s := range p.Members {
		if s.Advisory {
			continue
		}
		var ok bool
		if onset, ok = m.waveOnset(onset, s); !ok {
			return time.Time{}, false
		}
		if end, ok = m.waveNodeEnd(p, end, s); !ok {
			return time.Time{}, false
		}
	}
	if onset.Before(m.wave.started) {
		// An onset dated by the object, before the wave, would cancel
		// the hold the wave is there for.
		onset = m.wave.started
	}
	limit := scaleMaxHold
	if p.trafficLost || p.servingDown {
		limit = scaleMaxHoldServing
	}
	return minTime(end, onset.Add(limit)), true
}

// waveOnset folds the onset of failure s into onset; ok is false when
// s began well before the wave (scaleOnsetSlack), so is not its effect.
func (m *Manager) waveOnset(
	onset time.Time, s detection.Finding,
) (time.Time, bool) {
	if s.Since.IsZero() {
		return onset, true
	}
	if s.Since.Before(m.wave.started.Add(-scaleOnsetSlack)) {
		return time.Time{}, false
	}
	return minTime(onset, s.Since), true
}

// waveNodeEnd folds into end how long a failing node s of p may stay
// held (waveNodeFails); ok is false when the wave does not explain it.
// A failure that is not a node's own leaves end as it is.
func (m *Manager) waveNodeEnd(
	p *Incident, end time.Time, s detection.Finding,
) (time.Time, bool) {
	if s.Entity.Kind != kube.KindNode {
		return end, true
	}
	until, ok := m.waveNodeFails(p, s.Entity)
	if !ok {
		return time.Time{}, false
	}
	if !until.IsZero() {
		end = minTime(end, until)
	}
	return end, true
}

// sideModesOnly reports an incident whose failures are all scaling
// side-effect modes, with no change blamed but a node's coming or
// going: a release that broke is told at once, whatever the nodes do.
func sideModesOnly(p *Incident) bool {
	if cause := p.Cause; cause != nil && cause.Change != nil &&
		cause.Root.Kind != kube.KindNode {
		return false
	}
	found := false
	for _, s := range p.Members {
		if s.Advisory {
			continue
		}
		mode := s.Mode
		if mode == "" {
			mode = detection.ModeFor(s.Reason)
		}
		if !scaleSideModes[mode] {
			return false
		}
		found = true
	}
	return found
}

// waveLinked reports that the wave's nodes have to do with the
// failures of p that are not a node's own: behind each there is a pod
// the wave moved (movedPod), or one behind the root. A failure that
// stands for no pod at all (an Ingress) follows the others, and with
// no moved pod anywhere there is no link: an outage on nodes the wave
// never touched is told at once.
func (m *Manager) waveLinked(p *Incident) bool {
	if m.movedPodBehind(p.Root) {
		return true
	}
	linked, others := false, false
	for _, s := range p.Members {
		if s.Advisory || s.Entity.Kind == kube.KindNode {
			continue
		}
		others = true
		pods := podsBehind(m.model, s.Entity)
		switch {
		case len(pods) == 0:
		case m.anyMoved(pods):
			linked = true
		default:
			return false
		}
	}
	return linked || !others
}

// movedPodBehind reports a pod behind id that the wave moved.
func (m *Manager) movedPodBehind(id inventory.EntityID) bool {
	return id.Kind != kube.KindNode &&
		m.anyMoved(podsBehind(m.model, id))
}

func (m *Manager) anyMoved(pods []inventory.EntityID) bool {
	for _, pod := range pods {
		if m.movedPod(pod) {
			return true
		}
	}
	return false
}

// movedPod reports a pod the wave has to do with: it has no node yet,
// it is on a node the wave removed, is removing or added, or it was
// created no earlier than scaleLinkSlack before the wave.
func (m *Manager) movedPod(pod inventory.EntityID) bool {
	e, ok := m.model.Entity(pod)
	if !ok {
		return false
	}
	if a, ok := e.Attribute(kube.AttrCreated); ok &&
		!a.Value.AsTime().Before(m.wave.started.Add(-scaleLinkSlack)) {
		return true
	}
	nodes := m.model.Related(pod, inventory.RunsOn, inventory.Outgoing)
	for _, id := range nodes {
		if _, ok := m.waveNode(id); ok {
			return true
		}
	}
	return len(nodes) == 0
}

// waveNodeFails reports a node's own failure the scaling explains, and
// until when. It counts only for a node the wave touched: one removed
// or being removed (cordoned, deleting, or found draining by its
// detector, which also reads the autoscalers' removal taints), or one
// that joined during the wave, and that one only for scaleJoinGrace: a
// new node not ready after it is a broken node. A node that fails
// without being taken out is a lost node.
func (m *Manager) waveNodeFails(
	p *Incident, id inventory.EntityID,
) (time.Time, bool) {
	draining := detection.Key{Entity: id, Reason: reasons.NodeDraining}
	if _, ok := p.Members[draining]; ok {
		return time.Time{}, true
	}
	return m.waveNode(id)
}

// waveNode reports a node the wave removed, is removing or added. For
// one it only added, until is scaleJoinGrace after it was first seen;
// zero for the others.
func (m *Manager) waveNode(
	id inventory.EntityID,
) (until time.Time, ok bool) {
	node, found := m.model.Entity(id)
	if !found {
		return time.Time{}, true
	}
	for _, name := range []string{kube.AttrUnschedulable, kube.AttrDeleting} {
		if a, ok := node.Attribute(name); ok {
			if v, _ := a.Value.AsBool(); v {
				return time.Time{}, true
			}
		}
	}
	if node.FirstSeen.Before(m.wave.started) {
		return time.Time{}, false
	}
	return node.FirstSeen.Add(scaleJoinGrace), true
}

// pruneScaleHolds forgets the holds of incidents that stopped
// settling, so the set stays as small as the settling incidents.
func (m *Manager) pruneScaleHolds() {
	for id := range m.scaleHolds {
		if p := m.incidents[id]; p == nil || p.State != Settling {
			delete(m.scaleHolds, id)
		}
	}
}

// ScalingHeldText is the plain reason a held page gives.
const ScalingHeldText = "Paging held: cluster is scaling"

// NonProductionText is the plain reason of a tier the non-production
// setting kept from paging.
const NonProductionText = "Not paged: non-production cluster"

// NonProduction reports whether the cluster is set as not production,
// so nothing in it pages.
func (m *Manager) NonProduction() bool {
	return m.cfg.NonProduction
}
