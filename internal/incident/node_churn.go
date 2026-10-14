package incident

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Node churn. When a node is drained, terminated or lost, its pods are
// created again elsewhere and are not ready for a while: readiness,
// endpoints and scheduling blip, and clear by themselves. An incident
// that was announced and resolved lately would reopen on that blip and
// say "failing again", though nothing came back: the pods only moved.
//
// So a finding about pods that a node in transition displaced is held,
// like boot noise, for churnStart after the displacement. If it still
// fails then, it is a failure on its own and opens a new incident
// without the tier of the old one. Only a failure on settled
// infrastructure continues an earlier incident.

// churnStart is how long displaced pods get to become ready: the
// replacement grace of the detectors, as no start time is known here.
const churnStart = 5 * time.Minute

// churnMaxHold caps one finding's hold, so a cluster that keeps
// replacing nodes cannot silence a failure for long. It is also how long
// after a displacement a failure still counts as its blip.
const churnMaxHold = 15 * time.Minute

// nodeChurn remembers the nodes seen and whether each was leaving, when
// a node last started leaving or was removed, and the findings held.
type nodeChurn struct {
	seen map[inventory.EntityID]bool // value: the node was leaving
	at   time.Time
	held map[detection.Key]heldFinding
	// baselined is set by the first look at a model that has nodes. What
	// that look finds is the state kwatch started in, not a change.
	baselined bool
	// restored lists the nodes that restored incidents stood on.
	restored []inventory.EntityID
	// scans counts the looks made, so tests can prove a quiet tick
	// does not scan the nodes.
	scans int
}

// heldFinding is a finding waiting for its pods to settle.
type heldFinding struct {
	finding detection.Finding
	where   placement
	since   time.Time
}

// observe records a node that left the model, or started leaving it
// (not ready, cordoned or being deleted), as a displacement now. Only
// the change counts: a node that stays cordoned for days is not a new
// displacement on every look, and its pods settle after churnMaxHold.
//
// The first look at a model that has nodes only sets the baseline: nodes
// already leaving then began at an unknown time, so they are not fresh.
// A node that restored incidents stood on and that is gone by then was
// removed while kwatch was down, and counts as removed now.
func (c *nodeChurn) observe(model inventory.Reader, now time.Time) {
	if model == nil {
		return
	}
	c.scans++
	current := nodesLeaving(model)
	if !c.baselined {
		c.baseline(current, now)
		return
	}
	for id, leaving := range current {
		if leaving && !c.seen[id] {
			c.at = now
		}
	}
	for id := range c.seen {
		if _, still := current[id]; !still {
			c.at = now
		}
	}
	c.seen = current
}

// touchesNode reports whether an update names a node entity. The entity
// an update is about is always in dirty or in a transition.
func touchesNode(
	dirty []inventory.EntityID, transitions []detection.Transition,
) bool {
	for _, id := range dirty {
		if id.Kind == kube.KindNode {
			return true
		}
	}
	for _, t := range transitions {
		if t.Finding.Entity.Kind == kube.KindNode {
			return true
		}
	}
	return false
}

// observeNodes looks at the nodes when one changed since the last look,
// or when there is no baseline yet. Expiry of a displacement needs no
// look: it is read from the clock in displacedPods.
func (m *Manager) observeNodes(model inventory.Reader, now time.Time) {
	if !m.nodesDirty && m.churn.baselined {
		return
	}
	m.churn.observe(model, now)
	m.nodesDirty = false
}

// baseline takes the first look at a model that has nodes.
func (c *nodeChurn) baseline(
	current map[inventory.EntityID]bool, now time.Time,
) {
	if len(current) == 0 {
		return // the nodes are not in the model yet
	}
	c.baselined = true
	for _, id := range c.restored {
		if _, there := current[id]; !there {
			c.at = now
		}
	}
	c.restored, c.seen = nil, current
}

// knowNode remembers that a restored incident stood on node id, so its
// absence at the first look is a removal.
func (c *nodeChurn) knowNode(id inventory.EntityID) {
	if id.Kind == kube.KindNode && !c.baselined {
		c.restored = append(c.restored, id)
	}
}

// nodesLeaving maps every node in the model to whether it is leaving. It
// reads three attributes in bulk instead of copying each node.
func nodesLeaving(model inventory.Reader) map[inventory.EntityID]bool {
	ids := model.Entities(kube.KindNode)
	out := make(map[inventory.EntityID]bool, len(ids))
	spaces := map[string]bool{}
	for _, id := range ids {
		out[id] = false
		spaces[id.Namespace] = true
	}
	for space := range spaces {
		for _, name := range []string{kube.AttrReady, kube.AttrUnschedulable,
			kube.AttrDeleting} {
			for id, v := range model.AttributeIn(kube.KindNode, space, name) {
				b, isBool := v.AsBool()
				if isBool && b == (name != kube.AttrReady) {
					out[id] = true
				}
			}
		}
	}
	return out
}

// nodeLeaving reports a node that is not ready, cordoned or deleted.
func nodeLeaving(node inventory.Entity) bool {
	if ready, ok := node.Attribute(kube.AttrReady); ok {
		if v, isBool := ready.Value.AsBool(); isBool && !v {
			return true
		}
	}
	for _, name := range []string{kube.AttrUnschedulable, kube.AttrDeleting} {
		if a, ok := node.Attribute(name); ok {
			if v, _ := a.Value.AsBool(); v {
				return true
			}
		}
	}
	return false
}

// displacedPods reports whether a pod behind the entities was displaced
// by the churn: created since the latest displacement, or still on a
// node that is leaving.
func (c *nodeChurn) displacedPods(
	model inventory.Reader, now time.Time, ids ...inventory.EntityID,
) bool {
	if model == nil || c.at.IsZero() || now.Sub(c.at) > churnMaxHold {
		return false
	}
	for _, id := range ids {
		for _, pod := range podsBehind(model, id) {
			if c.displaced(model, pod) {
				return true
			}
		}
	}
	return false
}

func (c *nodeChurn) displaced(
	model inventory.Reader, pod inventory.EntityID,
) bool {
	e, ok := model.Entity(pod)
	if !ok {
		return false
	}
	if a, ok := e.Attribute(kube.AttrCreated); ok &&
		!a.Value.AsTime().Before(c.at) {
		return true
	}
	for _, id := range model.Related(pod, inventory.RunsOn,
		inventory.Outgoing) {
		if node, ok := model.Entity(id); ok && nodeLeaving(node) {
			return true
		}
	}
	return false
}

// podsBehind lists the pods a failing entity stands for: the pod itself,
// the pods a controller owns, or the pods a Service selects.
func podsBehind(
	model inventory.Reader, id inventory.EntityID,
) []inventory.EntityID {
	if id.Kind == kube.KindPod {
		return []inventory.EntityID{id}
	}
	if pods := kube.OwnedPods(model, id); len(pods) > 0 {
		return pods
	}
	if e, ok := model.Entity(id); ok && id.Kind == kube.KindService {
		return kube.SelectedPods(model, e)
	}
	return nil
}

// holdUntil is when a finding about displaced pods stops being held, or
// the zero time when it is not held now.
func (m *Manager) holdUntil(
	now time.Time, s detection.Finding, root inventory.EntityID,
) time.Time {
	c := &m.churn
	if !c.displacedPods(m.model, now, s.Entity, root) {
		return time.Time{}
	}
	until := c.at.Add(churnStart)
	if h, ok := c.held[s.Key()]; ok {
		until = minTime(until, h.since.Add(churnMaxHold))
	}
	if !now.Before(until) {
		return time.Time{}
	}
	return until
}

// heldForChurn holds s when an announced incident of its root ended
// lately, and it is only a displaced pod's blip. Any other finding goes
// through: the detectors already give boot noise its own grace.
func (m *Manager) heldForChurn(
	now time.Time, s detection.Finding, where placement,
) bool {
	prev := m.lookup(where.root)
	if prev == nil || !reopenable(prev, now, s.Mode) ||
		m.holdUntil(now, s, where.root).IsZero() {
		return false
	}
	if m.churn.held == nil {
		m.churn.held = map[detection.Key]heldFinding{}
	}
	key := s.Key()
	if h, ok := m.churn.held[key]; ok {
		h.finding, h.where = s, where
		m.churn.held[key] = h
	} else {
		m.churn.held[key] = heldFinding{s, where, now}
	}
	return true
}

// reopenRefused reports that a failure of a resolved incident's root
// comes from pods a node displaced after it resolved: it is not the
// same failure coming back, so it does not continue that incident.
func (m *Manager) reopenRefused(
	now time.Time, prev *Incident, s detection.Finding,
	root inventory.EntityID,
) bool {
	return m.churn.at.After(prev.Resolved) &&
		m.churn.displacedPods(m.model, now, s.Entity, root)
}

// releaseChurn attaches the held findings whose hold is over. A finding
// that cleared meanwhile was dropped by detach.
func (m *Manager) releaseChurn(now time.Time) {
	m.observeNodes(m.model, now)
	keys := make([]detection.Key, 0, len(m.churn.held))
	for key := range m.churn.held {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		return a.Entity.String()+a.Reason < b.Entity.String()+b.Reason
	})
	released := false
	for _, key := range keys {
		h := m.churn.held[key]
		if !m.holdUntil(now, h.finding, h.where.root).IsZero() {
			continue
		}
		m.attach(now, h.finding, h.where)
		delete(m.churn.held, key)
		released = true
	}
	if released {
		m.refresh(m.model, now)
	}
}

// churnWakes lists when held findings need a tick.
func (m *Manager) churnWakes() []time.Time {
	var out []time.Time
	for _, h := range m.churn.held {
		out = append(out, minTime(m.churn.at.Add(churnStart),
			h.since.Add(churnMaxHold)))
	}
	return out
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
