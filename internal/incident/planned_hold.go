package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// plannedSlack is how long before a scheduled scale-down's first
// change an incident may open and still be its effect: nodes switched
// off can stop reporting before the workloads are scaled.
const plannedSlack = 5 * time.Minute

// plannedOwnerHops bounds the owner chain followed from a pod to the
// workload a scale-down scaled.
const plannedOwnerHops = 4

// plannedHoldEnd is when the hold of a settling incident that a
// scheduled scale-down explains ends; ok is false when none does.
// Every failure of the incident must be one the scale-down explains:
// a node it removed, a workload it scaled to zero or what depends on
// one (the incident's root or cause is one), or a side effect of the
// scaling (scaleSideModes) on one. The hold lasts until PlannedGrace
// after the scale-up that followed was over: what still fails then did
// not come back, and is told as usual. One that clears meanwhile
// closes without a message.
func (m *Manager) plannedHoldEnd(p *Incident) (time.Time, bool) {
	if p.State != Settling || m.model == nil {
		return time.Time{}, false
	}
	down, end, ok := m.plannedDown(p.Opened)
	if !ok || !m.plannedExplains(p, down) {
		return time.Time{}, false
	}
	return end, true
}

// plannedSettle is how long a settling incident a scheduled scale-down
// holds settles: until its hold ends, and never less than settle.
func (m *Manager) plannedSettle(
	p *Incident, settle time.Duration,
) time.Duration {
	end, ok := m.plannedHoldEnd(p)
	if !ok {
		return settle
	}
	return max(settle, end.Sub(p.Opened))
}

// plannedHeld reports a settling incident a scheduled scale-down holds
// at now.
func (m *Manager) plannedHeld(p *Incident, now time.Time) bool {
	end, ok := m.plannedHoldEnd(p)
	return ok && now.Before(end)
}

// plannedDown is the latest scale-down begun no later than plannedSlack
// after opened, with when its effect ends, if it is scheduled and its
// effect still covers opened.
func (m *Manager) plannedDown(
	opened time.Time,
) (PlannedOp, time.Time, bool) {
	h := m.plan.history
	for i := len(h) - 1; i >= 0; i-- {
		op := h[i]
		if !op.Down || op.At.After(opened.Add(plannedSlack)) {
			continue
		}
		end := h.effectEnd(op)
		if _, ok := h.scheduled(op); !ok || !opened.Before(end) {
			return PlannedOp{}, time.Time{}, false
		}
		return op, end, true
	}
	return PlannedOp{}, time.Time{}, false
}

// plannedExplains reports an incident whose every failure the
// scale-down down explains.
func (m *Manager) plannedExplains(p *Incident, down PlannedOp) bool {
	rooted := m.plannedSubject(p.Root, down) ||
		(p.Cause != nil && m.plannedSubject(p.Cause.Root, down))
	found := false
	for _, s := range p.Members {
		if s.Advisory {
			continue
		}
		found = true
		if s.Entity.Kind == kube.KindNode {
			if !m.plan.down[s.Entity] {
				return false
			}
			continue
		}
		if !rooted && !m.plannedSide(s, down) {
			return false
		}
	}
	return found
}

// plannedSubject reports a node the scale-down removed or a workload it
// scaled, or a pod or ReplicaSet of one.
func (m *Manager) plannedSubject(id inventory.EntityID, down PlannedOp) bool {
	if id.Kind == kube.KindNode {
		return m.plan.down[id]
	}
	for range plannedOwnerHops {
		if m.plan.down[id] || slices.Contains(down.Workloads, id.String()) {
			return true
		}
		owner, ok := inventory.Owner(m.model, id)
		if !ok {
			return false
		}
		id = owner
	}
	return false
}

// plannedSide reports a scaling side effect (scaleSideModes) on what
// the scale-down touched: its own subject, or pods on a node it
// removed.
func (m *Manager) plannedSide(s detection.Finding, down PlannedOp) bool {
	mode := s.Mode
	if mode == "" {
		mode = detection.ModeFor(s.Reason)
	}
	if !scaleSideModes[mode] {
		return false
	}
	if m.plannedSubject(s.Entity, down) {
		return true
	}
	for _, pod := range podsBehind(m.model, s.Entity) {
		for _, node := range m.model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			if m.plan.down[node] {
				return true
			}
		}
	}
	return false
}

// PlannedOps returns the history of mass scale-downs and scale-ups, for
// the digest state that persists it.
func (m *Manager) PlannedOps() []PlannedOp {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ClonePlanned(m.plan.history)
}

// RestorePlannedOps puts back the history an earlier run saved.
func (m *Manager) RestorePlannedOps(ops []PlannedOp) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plan.history = plannedHistory(ClonePlanned(ops))
}
