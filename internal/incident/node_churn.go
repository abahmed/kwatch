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

// nodeChurn remembers the nodes seen, when a node last left or was in
// transition, and the findings held for it.
type nodeChurn struct {
	seen map[inventory.EntityID]bool
	at   time.Time
	held map[detection.Key]heldFinding
}

// heldFinding is a finding waiting for its pods to settle.
type heldFinding struct {
	finding detection.Finding
	where   placement
	since   time.Time
}

// observe records a node that left the model or is leaving it (not
// ready, cordoned or being deleted) as a displacement now.
func (c *nodeChurn) observe(model inventory.Reader, now time.Time) {
	if model == nil {
		return
	}
	current := map[inventory.EntityID]bool{}
	for _, id := range model.Entities(kube.KindNode) {
		current[id] = true
		if node, ok := model.Entity(id); ok && nodeLeaving(node) {
			c.at = now
		}
	}
	for id := range c.seen {
		if !current[id] {
			c.at = now
		}
	}
	c.seen = current
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
	m.churn.observe(m.model, now)
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
