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
	// Checked was added with the "checked and found healthy" wording;
	// older records restore without it and the next explanation fills
	// it again.
	Checked []string `json:",omitempty"`
	// Considered was added with the audit log's alternatives; older
	// records restore without it.
	Considered []string `json:",omitempty"`
	// Reminded was added with the weekly reminder; an older record
	// restores as never reminded, which costs at most one reminder.
	Reminded time.Time `json:",omitempty"`
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
	// Replaced says the revision swapped a known cause for another.
	Replaced bool `json:",omitempty"`
	// RootReasons are the root's own reasons seen so far, so a restart
	// does not re-report a condition the incident already covered. Older
	// records restore without them and rebuild the set from the members.
	RootReasons []string `json:",omitempty"`
	// AlertKey was added so paging alerts keep their identity across
	// restarts. An older announced record restores with the key derived
	// from its root and mode, as a new announcement would get it.
	AlertKey string `json:",omitempty"`
	// Paged, PagedKnown, DigestedAt, RolledUp, PodPeak and PageHeld
	// were added with paging-only resolves, chronic reminders and
	// repeated-page suppression. A record without PagedKnown restores an
	// announced incident as paged, which keeps the old behaviour: its
	// resolve reaches every provider.
	Paged      bool      `json:",omitempty"`
	PagedKnown bool      `json:",omitempty"`
	DigestedAt time.Time `json:",omitempty"`
	RolledUp   bool      `json:",omitempty"`
	PodPeak    int       `json:",omitempty"`
	PageHeld   bool      `json:",omitempty"`
	// Modes are every failure mode the incident's members had, so a
	// restart does not make the same failure of a root look new. Older
	// records restore without them.
	Modes []detection.Mode `json:",omitempty"`
	// ReopenedAt was added with true re-opens: a restart keeps a pending
	// "failing again" update. Older records restore without it.
	ReopenedAt time.Time `json:",omitempty"`
	// RepeatCount was added with typed repeat counts; older records
	// restore with zero and the next reopen sets it.
	RepeatCount int `json:",omitempty"`
	// Unheard marks an announced incident that no message ever reached
	// (held, then resolved, or out of scope). It is the inverse of what
	// is kept in memory, so older records restore as heard, except a
	// held one: ToldKnown is written by every record that has Unheard,
	// and a held record without it was saved before Unheard existed, when
	// a held announcement was never told.
	Unheard   bool `json:",omitempty"`
	ToldKnown bool `json:",omitempty"`
	// StagePeak and AnnouncedRoute were added with the restart review.
	// StagePeak keeps a crash loop that began after the announcement
	// from reading as news again after a restart; AnnouncedRoute is how
	// the first announcement was routed, which the resolve reuses. Older
	// records restore without them.
	StagePeak      uint8           `json:",omitempty"`
	AnnouncedRoute *AnnouncedRoute `json:",omitempty"`
	// Ack is the acknowledgement the thread was told about.
	Ack *Ack `json:",omitempty"`
	// Demoted was added with lowering the tier of an announced incident.
	Demoted bool `json:",omitempty"`
	// HeldUpdate marks a material-change update that was waiting for its
	// investigation when the record was saved. Digest then holds the
	// fingerprint before that update, and the restore does not adopt the
	// current one, so the update is decided again.
	HeldUpdate bool `json:",omitempty"`
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
			Digest: p.savedDigest(), Timeline: p.Timeline, Scope: p.Scope,
			HeldUpdate: p.updateHeld,
			Held:       p.Held, SupersededBy: p.SupersededBy,
			Unverified: p.Unverified, CauseUnclear: p.CauseUnclear,
			Checked: p.Checked, Considered: p.Considered, Mode: p.Mode,
			Fix: p.Fix, History: p.History, Reminded: p.Reminded,
			ImpactPeak: p.impactPeak, Revised: p.Pending.revised,
			Replaced:  p.Pending.replaced,
			RevisedAt: p.Pending.revisedAt, AlertKey: p.AlertKey,
			RootReasons: p.rootReasonList(),
			Paged:       p.Delivery.paged, PagedKnown: true,
			DigestedAt: p.DigestedAt, RolledUp: p.Delivery.rolledUp,
			PodPeak: p.Delivery.podPeak, PageHeld: p.Delivery.pageHeld,
			ReopenedAt: p.Pending.reopenedAt, Modes: p.modeList(),
			RepeatCount: p.RepeatCount, Unheard: !p.sent.told,
			ToldKnown: true,
			StagePeak: uint8(p.stagePeak), AnnouncedRoute: p.AnnouncedRoute,
			Ack: p.Ack, Demoted: p.Delivery.demoted,
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
		if wasAnnounced(p) && !r.HeldUpdate {
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

// restoreSets rebuilds the failure modes and the root's own reasons.
func restoreSets(p *Incident, r Record) {
	for _, mode := range r.Modes {
		if p.modes == nil {
			p.modes = map[detection.Mode]struct{}{}
		}
		p.modes[mode] = struct{}{}
	}
	for _, reason := range r.RootReasons {
		if p.rootReasons == nil {
			p.rootReasons = map[string]struct{}{}
		}
		p.rootReasons[reason] = struct{}{}
	}
}

// savedDigest is the fingerprint to persist: the one before a held update
// is the one people were last told about.
func (p Incident) savedDigest() string {
	if p.updateHeld {
		return p.prevDigest
	}
	return p.Digest
}

// heard reports whether anyone was told of the incident before the
// restart. A held record from before ToldKnown existed was never told.
func (r Record) heard() bool {
	if r.Held && !r.ToldKnown {
		return false
	}
	return !r.Unheard
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
		Checked: r.Checked, Considered: r.Considered, Reminded: r.Reminded,
		Mode: r.Mode, Fix: r.Fix, History: r.History,
		Members:    make(map[detection.Key]detection.Finding),
		impactPeak: r.ImpactPeak, AlertKey: r.AlertKey,
		sent: restoredMark(len(r.Timeline), r.heard()), restored: true,
		DigestedAt: r.DigestedAt, RepeatCount: r.RepeatCount,
		stagePeak: stage(r.StagePeak), AnnouncedRoute: r.AnnouncedRoute,
		Ack: r.Ack,
		Delivery: Delivery{
			paged: r.Paged, rolledUp: r.RolledUp,
			podPeak: r.PodPeak, pageHeld: r.PageHeld,
			demoted: r.Demoted,
		},
		Pending: Pending{
			reopenedAt: r.ReopenedAt, revised: r.Revised,
			revisedAt: r.RevisedAt, replaced: r.Replaced,
		},
	}
	restoreSets(p, r)
	if !r.PagedKnown && wasAnnounced(p) {
		p.Delivery.MarkPaged()
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
