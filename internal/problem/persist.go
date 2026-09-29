package problem

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

// Record is the persisted form of a problem. Members are not stored: they
// are re-derived from the cluster after a restart and re-attach to the
// same problem ID, so nothing is announced twice.
type Record struct {
	ID              string
	Root            knowledge.EntityID
	Cause           *reason.Hypothesis `json:",omitempty"`
	Tier            Tier
	State           State
	Opened          time.Time
	Announced       time.Time
	RecoveringSince time.Time
	Resolved        time.Time
	Cycles          []time.Time
	Occurrences     []time.Time `json:",omitempty"`
	Revision        int
	Digest          string
	Timeline        []Event
}

// Export returns every problem as a record, for persistence.
func (m *Manager) Export() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.problems))
	for _, id := range m.sortedIDs() {
		p := m.problems[id].Snapshot()
		out = append(out, Record{
			ID: p.ID, Root: p.Root, Cause: p.Cause, Tier: p.Tier,
			State: p.State, Opened: p.Opened, Announced: p.Announced,
			RecoveringSince: p.RecoveringSince, Resolved: p.Resolved,
			Cycles: p.Cycles, Occurrences: p.Occurrences,
			Revision: p.Revision, Digest: p.Digest, Timeline: p.Timeline,
		})
	}
	return out
}

// Restore loads records after a restart. Until graceUntil, restored open
// problems without members are not moved to recovering: detectors need
// time to re-raise their signals after the model is rebuilt.
func (m *Manager) Restore(records []Record, graceUntil time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreGrace = graceUntil
	for _, r := range records {
		m.problems[r.ID] = &Problem{
			ID: r.ID, Root: r.Root, Cause: r.Cause, Tier: r.Tier,
			State: r.State, Opened: r.Opened, Announced: r.Announced,
			RecoveringSince: r.RecoveringSince, Resolved: r.Resolved,
			Cycles: r.Cycles, Occurrences: r.Occurrences,
			Revision: r.Revision, Digest: r.Digest, Timeline: r.Timeline,
			Members: make(map[signal.Key]signal.Signal),
		}
	}
}
