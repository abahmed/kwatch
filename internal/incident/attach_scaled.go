package incident

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An autoscaler reads its metrics from the ready pods of its target.
// When the target has no ready pod there is nothing to read: "cannot
// read its metrics" is a symptom of the workload's own failure, not a
// second problem. It joins the incident that already speaks for the
// workload, so one failure is told once. Without such an incident it
// stays an incident of its own, as before.

// joinScaledWorkload sends an autoscaler's metrics failure that
// nothing else explains to the live incident of its target, when the
// target has no ready replica. Once it joined, it stays while that
// incident lives: the metrics come back a little after the pods do.
// But not for good: when the target has had ready replicas for
// scaledSettle and the metrics still fail, the workload is not why,
// and releaseScaled gives the autoscaler an incident of its own.
func (m *Manager) joinScaledWorkload(
	s detection.Finding, where placement,
) placement {
	if s.Reason != reasons.FailedGetResourceMetric ||
		s.Entity.Kind != kube.KindHPA || where.root != s.Entity ||
		m.model == nil {
		return where
	}
	for _, target := range m.model.Related(s.Entity, inventory.Scales,
		inventory.Outgoing) {
		if p := m.scaledJoinTo(s.Key(), target); p != nil {
			return placement{root: p.Root}
		}
	}
	return where
}

// scaledJoinTo is the incident of target that the autoscaler finding
// key joins: one it already joined, or the workload's live incident
// while target has no ready replica. Nil when it joins none. A new
// join is remembered for releaseScaled.
func (m *Manager) scaledJoinTo(
	key detection.Key, target inventory.EntityID,
) *Incident {
	p := m.workloadIncident(target)
	if p == nil {
		return nil
	}
	_, held := m.scaled[key]
	if held && m.byMember[key] == p.ID {
		return p
	}
	if _, ready, ok := ReplicasOf(m.model, target); !ok || ready != 0 {
		return nil
	}
	if !held {
		if m.scaled == nil {
			m.scaled = map[detection.Key]time.Time{}
		}
		m.scaled[key] = time.Time{}
	}
	return p
}

// workloadIncident is the live incident with a real failure that
// speaks for workload: rooted at it, or holding a failure of it or of
// its pods. The lowest ID wins; nil when there is none.
func (m *Manager) workloadIncident(workload inventory.EntityID) *Incident {
	if p := m.lookup(workload); p != nil && p.State != Resolved &&
		hasFailingMember(p) {
		return p
	}
	for _, id := range m.sortedIDs() {
		p := m.incidents[id]
		if p.State == Resolved {
			continue
		}
		for key, f := range p.Members {
			if !f.Advisory && key.Entity.Kind != kube.KindHPA &&
				defaultRoot(m.model, key.Entity) == workload {
				return p
			}
		}
	}
	return nil
}

// scaledSettle is how long the target of a joined autoscaler must have
// ready replicas before its metrics failure stops being the workload's:
// the metrics need a scrape or two after the pods are back.
const scaledSettle = 5 * time.Minute

// releaseScaled ends the join of every autoscaler whose target has had
// ready replicas for scaledSettle: its metrics failure moves to an
// incident of its own, and the workload's incident can close.
func (m *Manager) releaseScaled(now time.Time) {
	keys := make([]detection.Key, 0, len(m.scaled))
	for key := range m.scaled {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Entity.String() < keys[j].Entity.String()
	})
	released := false
	for _, key := range keys {
		p := m.incidents[m.byMember[key]]
		if p == nil || p.State == Resolved || p.Root == key.Entity {
			delete(m.scaled, key)
			continue
		}
		since := m.scaledReadySince(key, now)
		m.scaled[key] = since
		if since.IsZero() || now.Sub(since) < scaledSettle {
			continue
		}
		delete(m.scaled, key)
		m.attach(now, p.Members[key], placement{root: key.Entity})
		released = true
		if m.byMember[key] == p.ID {
			// Kept back (keepsMember): the workload's root still
			// fails. Try again on a later tick, or the join would
			// never end and the workload's incident never close.
			m.scaled[key] = since
		}
	}
	if released {
		m.refresh(m.model, now)
	}
}

// scaledReadySince is since when the target of the joined autoscaler
// key has ready replicas, as far as the ticks saw; zero when it has
// none now.
func (m *Manager) scaledReadySince(
	key detection.Key, now time.Time,
) time.Time {
	if m.model == nil {
		return time.Time{}
	}
	for _, target := range m.model.Related(key.Entity, inventory.Scales,
		inventory.Outgoing) {
		if _, ready, ok := ReplicasOf(m.model, target); ok && ready > 0 {
			if since := m.scaled[key]; !since.IsZero() {
				return since
			}
			return now
		}
	}
	return time.Time{}
}

// scaledWakes lists when joined autoscalers are due for release.
func (m *Manager) scaledWakes() []time.Time {
	var out []time.Time
	for _, since := range m.scaled {
		if !since.IsZero() {
			out = append(out, since.Add(scaledSettle))
		}
	}
	return out
}
