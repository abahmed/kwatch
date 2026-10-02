package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Record is the persisted form of an incident. Members are not stored: they
// are re-derived from the cluster after a restart and re-attach to the
// incident of their root, so nothing is announced twice. ID and Root are
// independent: the ID is opaque and Root is the current root.
type Record struct {
	ID              string
	Root            inventory.EntityID
	Previous        string                 `json:",omitempty"`
	Cause           *rootcause.CauseRecord `json:",omitempty"`
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
	// Scope, Held and SupersededBy were added after the first format;
	// records without them restore as described in Restore.
	Scope        Scope  `json:",omitempty"`
	Held         bool   `json:",omitempty"`
	SupersededBy string `json:",omitempty"`
	// Unverified and CauseUnclear were added later; older records
	// restore without them and the next explanation fills them again.
	Unverified   []string `json:",omitempty"`
	CauseUnclear bool     `json:",omitempty"`
	// Mode, Fix and History were added for recurrence memory; older
	// records restore without them.
	Mode    detection.Mode `json:",omitempty"`
	Fix     Fix            `json:",omitempty"`
	History []Occurrence   `json:",omitempty"`
	// ImpactPeak, Revised and RevisedAt were added so a restart neither
	// re-reports impact that already peaked nor drops a pending "cause
	// revised" update. Older records restore without them: the peak is
	// rebuilt from the current impact and no revision is pending.
	ImpactPeak int       `json:",omitempty"`
	Revised    bool      `json:",omitempty"`
	RevisedAt  time.Time `json:",omitempty"`
	// AlertKey was added so paging alerts keep their identity across
	// restarts. An older announced record restores with the key derived
	// from its root and mode, as a new announcement would get it.
	AlertKey string `json:",omitempty"`
}

// Export returns every incident as a record, for persistence.
func (m *Manager) Export() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.incidents))
	for _, id := range m.sortedIDs() {
		p := m.incidents[id].Snapshot()
		out = append(out, Record{
			ID: p.ID, Root: p.Root, Previous: p.Previous, Cause: p.Cause,
			Tier: p.Tier, State: p.State, Opened: p.Opened,
			Announced: p.Announced, RecoveringSince: p.RecoveringSince,
			Resolved: p.Resolved, Cycles: p.Cycles,
			Occurrences: p.Occurrences, Revision: p.Revision,
			Digest: p.Digest, Timeline: p.Timeline, Scope: p.Scope,
			Held: p.Held, SupersededBy: p.SupersededBy,
			Unverified: p.Unverified, CauseUnclear: p.CauseUnclear,
			Mode: p.Mode, Fix: p.Fix, History: p.History,
			ImpactPeak: p.impactPeak, Revised: p.revised,
			RevisedAt: p.revisedAt, AlertKey: p.AlertKey,
		})
	}
	return out
}

// Restore loads records after a restart. Until graceUntil, restored open
// incidents without members are not moved to recovering: detectors need
// time to re-raise their findings after the model is rebuilt. Without a
// live record there is nothing to wait for, and no grace applies. New IDs
// continue after the highest restored sequence number, with its nonce.
func (m *Manager) Restore(records []Record, graceUntil time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	newest := 0
	for _, r := range records {
		p := restored(r)
		if p.State != Resolved {
			// Only restored live incidents wait for their findings;
			// with none, such as on a cold start, new incidents
			// recover as usual.
			m.restoreGrace = graceUntil
		}
		m.incidents[p.ID] = p
		m.indexRestored(p)
		if wasAnnounced(p) {
			m.refingerprint = append(m.refingerprint, p.ID)
		}
		if seq := idSequence(p.ID); seq > newest {
			newest = seq
			if nonce := idNonce(p.ID); nonce != "" {
				m.nonce = nonce
			}
		}
	}
	m.seq = max(m.seq, newest)
	m.indexResolvedRestored()
}

// restored rebuilds an incident from its record.
//
// A held announcement never reached delivery, so it restores as settling
// and is announced again. A record from before scope was persisted that
// was announced is treated as delivered, so its resolve is not dropped.
func restored(r Record) *Incident {
	p := &Incident{
		ID: r.ID, Root: r.Root, Previous: r.Previous,
		Cause: restoredCause(r.Cause),
		Tier:  r.Tier, State: r.State, Opened: r.Opened,
		Announced: r.Announced, RecoveringSince: r.RecoveringSince,
		Resolved: r.Resolved, Cycles: r.Cycles, Occurrences: r.Occurrences,
		Revision: r.Revision, Digest: r.Digest, Timeline: r.Timeline,
		Scope: r.Scope, SupersededBy: r.SupersededBy,
		Unverified: r.Unverified, CauseUnclear: r.CauseUnclear,
		Mode: r.Mode, Fix: r.Fix, History: r.History,
		Members:    make(map[detection.Key]detection.Finding),
		impactPeak: r.ImpactPeak, revised: r.Revised,
		revisedAt: r.RevisedAt, AlertKey: r.AlertKey,
		sent: restoredMark(len(r.Timeline)),
	}
	if p.AlertKey == "" && wasAnnounced(p) {
		p.AlertKey = alertKey(p.Root, p.Mode)
	}
	if r.Held {
		p.State, p.Announced, p.Digest = Settling, time.Time{}, ""
		p.Scope = ScopeUnknown
	}
	if p.Scope == ScopeUnknown && wasAnnounced(p) {
		p.Scope = ScopeIn
	}
	return p
}
