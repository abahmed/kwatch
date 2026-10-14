package incident

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A node can be named as what failing workloads have in common while it
// still looks healthy. If the workloads then go their own ways and the
// node really fails a little later, the suspicion was right: the
// workloads rejoin the incident people were told about, instead of
// announcing the same failures again beside it.

// leftNode is where a finding was told about and when it left.
type leftNode struct {
	id string
	at time.Time
}

// noteLeft remembers that a finding left an announced incident rooted at
// a node, so it can rejoin if that node fails.
func (m *Manager) noteLeft(now time.Time, old *Incident, key detection.Key) {
	if old.Root.Kind != kube.KindNode || !wasAnnounced(old) {
		return
	}
	m.left[key] = leftNode{id: old.ID, at: now}
}

// nodeLost reports an incident whose root node reports it is not ready or
// not reporting now.
func nodeLost(p *Incident) bool {
	for key, s := range p.Members {
		if key.Entity == p.Root && !s.Advisory &&
			(key.Reason == reasons.NodeNotReady ||
				key.Reason == reasons.NodeHeartbeatStale) {
			return true
		}
	}
	return false
}

// retakeSuspect sends a finding that left an announced node incident,
// and has no cause now, back to it while that node is lost.
func (m *Manager) retakeSuspect(
	now time.Time, s detection.Finding, where placement,
) placement {
	key := s.Key()
	rec, ok := m.left[key]
	if !ok {
		return where
	}
	p := m.incidents[rec.id]
	if p == nil || p.State == Resolved ||
		now.Sub(rec.at) > RepageWindow {
		delete(m.left, key)
		return where
	}
	if where.cause != nil || !wasAnnounced(p) || !nodeLost(p) {
		return where
	}
	return placeIn(p, where)
}

// recallSuspects brings the findings that left an announced node incident
// back to it as soon as the node is lost, without waiting for their own
// areas to be solved again.
func (m *Manager) recallSuspects(now time.Time) {
	keys := make([]detection.Key, 0, len(m.left))
	for key := range m.left {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Entity.String()+keys[i].Reason <
			keys[j].Entity.String()+keys[j].Reason
	})
	for _, key := range keys {
		p := m.incidents[m.left[key].id]
		holder := m.incidents[m.byMember[key]]
		if p == nil || holder == nil || holder == p ||
			!wasAnnounced(p) || !nodeLost(p) {
			continue
		}
		if f, ok := holder.Members[key]; ok {
			m.attach(now, f, placeIn(p, placement{}))
		}
	}
}
