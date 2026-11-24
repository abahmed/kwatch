package incident

import (
	"sort"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Failures on brand-new nodes. Right after a node joins, its pods may
// fail for a minute or two (DNS not ready, network not set up, a
// dependency not reachable from it yet) and then recover. A failure
// that says a connection or lookup failed and whose failing pods all
// run on nodes younger than kube.YoungNodeWindow (3 minutes, not the
// 10-minute kube.BootWindow: a connection failure can be a real
// outage), while no pod of the same workloads fails on an older node
// and no incident of the same cause is open, is held until the youngest
// of those nodes is no longer young. A hold never moves later once
// set, so it ends at most kube.YoungNodeWindow after it began. If it
// still fails then, it is reported as usual; if it cleared, nothing is
// said. A failure of a pod on an older node with the same cause opens
// the incident at once and releases what was held, so a real outage is
// never hidden. When every node is new (a cold start) there is no older
// node to compare with, and the short window is the only bound. Only
// node age and placement are read: nothing about how nodes are added.

// youngHeld is a finding held while its pods' nodes are new.
type youngHeld struct {
	finding detection.Finding
	where   placement
	until   time.Time
}

// heldOnYoungNode holds s while every failing pod behind it runs on a
// node that joined less than kube.YoungNodeWindow ago (see
// youngHoldUntil). A finding already held keeps its end, and is never
// held again once that end has passed: a pod on a newer node never
// makes the hold longer.
func (m *Manager) heldOnYoungNode(
	now time.Time, s detection.Finding, where placement,
) bool {
	key := s.Key()
	if _, member := m.byMember[key]; member || m.model == nil ||
		s.Advisory || where.root.Kind == kube.KindNode {
		return false
	}
	if p := m.lookup(where.root); p != nil && p.State != Resolved {
		return false
	}
	held, wasHeld := m.churn.young[key]
	until := youngHoldUntil(m.model, now, s.Entity)
	if wasHeld && held.until.Before(until) {
		until = held.until
	}
	if !now.Before(until) || !failsToConnect(m.model, s) {
		delete(m.churn.young, key)
		return false
	}
	if m.churn.young == nil {
		m.churn.young = map[detection.Key]youngHeld{}
	}
	m.churn.young[key] = youngHeld{finding: s, where: where, until: until}
	return true
}

// failsToConnect reports a failure whose own words, or its pods'
// containers' own log lines, say a connection or a lookup failed: what a
// node whose network or DNS is not ready yet causes. Any other failure
// on a new node (a missing file, a bad setting) is the app's and is
// never held.
func failsToConnect(model inventory.Reader, s detection.Finding) bool {
	for _, e := range s.Evidence {
		// The kubelet's probe message is the kubelet's call to the
		// pod, never one the pod makes.
		if e.Label != "probe" && explain.ConnectionFailure(e.Value) {
			return true
		}
	}
	for _, pod := range failurePods(model, s.Entity) {
		for _, c := range model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			for _, attr := range []string{kube.AttrLastErrorLine,
				kube.AttrProbeLogLine, kube.AttrLastMessage} {
				line, _ := model.Attribute(c, attr)
				if explain.ConnectionFailure(line.Value.AsText()) {
					return true
				}
			}
		}
	}
	return false
}

// youngHoldUntil is when the newest node of id's failing pods stops
// being young, or zero when id is not held: a pod behind it fails on an
// older node, a pod of the same workloads on an older node fails, or no
// failing pod is behind it.
func youngHoldUntil(
	model inventory.Reader, now time.Time, id inventory.EntityID,
) time.Time {
	var until time.Time
	failing := 0
	for _, pod := range failurePods(model, id) {
		if !podFailingNow(model, pod) {
			continue
		}
		failing++
		remaining := podNodeYoung(model, pod, now)
		if remaining <= 0 || oldSiblingFails(model, pod, now) {
			return time.Time{}
		}
		if at := now.Add(remaining); at.After(until) {
			until = at
		}
	}
	if failing == 0 {
		return time.Time{}
	}
	return until
}

// failurePods are the pods a finding is about: a container's pod, or
// the pods behind a pod, a controller or a Service.
func failurePods(
	model inventory.Reader, id inventory.EntityID,
) []inventory.EntityID {
	if id.Kind == kube.KindContainer {
		return model.Related(id, inventory.PartOf, inventory.Outgoing)
	}
	return podsBehind(model, id)
}

// podNodeYoung is how much longer the pod's node counts as new for
// this hold (kube.YoungNodeWindow).
func podNodeYoung(
	model inventory.Reader, pod inventory.EntityID, now time.Time,
) time.Duration {
	nodes := model.Related(pod, inventory.RunsOn, inventory.Outgoing)
	if len(nodes) == 0 {
		return 0
	}
	return kube.NodeYoungWithin(model, nodes[0], now, kube.YoungNodeWindow)
}

// oldSiblingFails reports a failing pod of pod's workload on a node
// that is not new: the failure is not the new node's.
func oldSiblingFails(
	model inventory.Reader, pod inventory.EntityID, now time.Time,
) bool {
	for _, owner := range model.Related(pod, inventory.OwnedBy,
		inventory.Outgoing) {
		for _, sibling := range kube.OwnedPods(model, owner) {
			if sibling != pod && podFailingNow(model, sibling) &&
				podNodeYoung(model, sibling, now) <= 0 {
				return true
			}
		}
	}
	return false
}

// podFailingNow reports a pod that is not ready while running, or a
// container of which waits or restarted.
func podFailingNow(model inventory.Reader, pod inventory.EntityID) bool {
	if ready, ok := model.Attribute(pod, kube.AttrReady); ok {
		if up, isBool := ready.Value.AsBool(); isBool && !up {
			return true
		}
	}
	for _, c := range model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		state, _ := model.Attribute(c, kube.AttrState)
		if state.Value.AsText() == "waiting" {
			return true
		}
	}
	return false
}

// releaseYoung attaches the held findings whose nodes are no longer
// new, and every held finding whose cause now has an open incident. A
// finding that cleared meanwhile was dropped by detach. The record stays
// until attach has run, so the finding is not held a second time.
func (m *Manager) releaseYoung(now time.Time) {
	for _, key := range m.youngKeys() {
		// Attaching one may release others (releaseYoungFor).
		h, held := m.churn.young[key]
		if !held {
			continue
		}
		p := m.lookup(h.where.root)
		if now.Before(h.until) && (p == nil || p.State == Resolved) {
			continue
		}
		m.attach(now, h.finding, h.where)
		delete(m.churn.young, key)
	}
}

// youngKeys are the held findings' keys in a stable order.
func (m *Manager) youngKeys() []detection.Key {
	keys := make([]detection.Key, 0, len(m.churn.young))
	for key := range m.churn.young {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		return a.Entity.String()+a.Reason < b.Entity.String()+b.Reason
	})
	return keys
}

// releaseYoungFor attaches at once the findings held for root, now that
// a failure of root on an older node opened its incident.
func (m *Manager) releaseYoungFor(now time.Time, root inventory.EntityID) {
	for _, key := range m.youngKeys() {
		if h, held := m.churn.young[key]; held && h.where.root == root {
			delete(m.churn.young, key)
			m.attach(now, h.finding, h.where)
		}
	}
}

// youngWakes lists when held findings' nodes stop being new.
func (m *Manager) youngWakes() []time.Time {
	var out []time.Time
	for _, h := range m.churn.young {
		out = append(out, h.until)
	}
	return out
}

// withNodeAge adds the node-age fact to a failure of pods that began
// failing less than kube.YoungNodeWindow after their node joined (the
// window the hold uses): the node and how many minutes after it joined
// they began.
func withNodeAge(
	model inventory.Reader, s detection.Finding,
) detection.Finding {
	if model == nil || s.Since.IsZero() ||
		evidenceOf(s, detection.EvidenceYoungNode) != "" {
		return s
	}
	for _, pod := range failurePods(model, s.Entity) {
		nodes := model.Related(pod, inventory.RunsOn, inventory.Outgoing)
		// A ready replica of a failing workload is not what fails.
		if len(nodes) == 0 || !podFailingNow(model, pod) {
			continue
		}
		created, ok := model.Attribute(nodes[0], kube.AttrCreated)
		joined := created.Value.AsTime()
		after := s.Since.Sub(joined)
		if !ok || joined.IsZero() || after < 0 ||
			after > kube.YoungNodeWindow {
			continue
		}
		s.Evidence = append(append([]detection.Evidence(nil),
			s.Evidence...), detection.Evidence{
			Label: detection.EvidenceYoungNode,
			Value: nodes[0].Name + " " +
				strconv.Itoa(int(after.Minutes()))})
		return s
	}
	return s
}

// evidenceOf is the value of a finding's evidence with label, or "".
func evidenceOf(s detection.Finding, label string) string {
	for _, e := range s.Evidence {
		if e.Label == label {
			return e.Value
		}
	}
	return ""
}
