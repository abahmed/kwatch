package incident

// Export returns every incident as a record, for persistence.
func (m *Manager) Export() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.incidents))
	for _, id := range m.sortedIDs() {
		out = append(out, recordOf(m.incidents[id].Snapshot()))
	}
	return m.markSequence(out)
}

// ExportOf returns incident id as a record, as Export would; false when
// there is no such incident. It copies that one incident only.
func (m *Manager) ExportOf(id string) (Record, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil {
		return Record{}, false
	}
	return m.markSequence([]Record{recordOf(p.Snapshot())})[0], true
}

// recordOf is the record of the snapshot p.
func recordOf(p Incident) Record {
	return Record{
		ID: p.ID, Root: p.Root, Previous: p.Previous, Cause: p.Cause,
		Tier: p.Tier, State: p.State, Opened: p.Opened,
		Announced: p.Announced, RecoveringSince: p.RecoveringSince,
		Resolved: p.Resolved, HeldSince: p.heldSince, Cycles: p.Cycles,
		Occurrences: p.Occurrences, Revision: p.Revision,
		Digest: p.savedDigest(), Timeline: p.Timeline, Scope: p.Scope,
		HeldUpdate: p.updateHeld, MemberKeys: savedKeys(&p),
		Held: p.Held, SupersededBy: p.SupersededBy,
		Unverified: p.Unverified, CauseUnclear: p.CauseUnclear,
		Checked: p.Checked, Considered: p.Considered, Mode: p.Mode,
		Fix: p.Fix, History: p.History, Reminded: p.Reminded,
		Reminders:  p.Reminders,
		ImpactPeak: p.impactPeak, Revised: p.Pending.revised,
		Replaced:  p.Pending.replaced,
		RevisedAt: p.Pending.revisedAt, AlertKey: p.AlertKey,
		RootReasons:     p.rootReasonList(),
		RestatedReasons: p.restatedReasonList(),
		ToldCauses:      p.causes.told, ToldCause: p.causes.last,
		ToldCauseKnown: p.causes.known,
		Paged:          p.Delivery.paged, PagedKnown: true,
		DigestedAt: p.DigestedAt, RolledUp: p.Delivery.rolledUp,
		PodPeak: p.Delivery.podPeak, PageHeld: p.Delivery.pageHeld,
		FlapPaged:  p.Delivery.flapPaged,
		ReopenedAt: p.Pending.reopenedAt, Modes: p.modeList(),
		RepeatCount: p.RepeatCount, Unheard: !p.sent.told,
		ToldKnown: true,
		StagePeak: uint8(p.stagePeak), AnnouncedRoute: p.AnnouncedRoute,
		Ack: p.Ack, Demoted: p.Delivery.demoted, Subject: p.Subject,
		AckIgnored: p.ackIgnored, AckAll: p.ackAll, PastRoots: p.pastRoots,
		FormerRoot: p.formerRoot, RerootedAt: p.rerootedAt,
		RejectionSeenAt: p.rejectionAt, Merged: p.Merged,
		Facts: p.Facts, Headline: p.Headline, DocsLink: p.DocsLink,
		Problem: p.Problem, ProblemFull: p.ProblemFull,
		Steps: p.Steps, Reasons: savedReasons(&p),
		Members: savedMembers(&p), Breaks: p.Journal.Breaks,
		LinkedChanges: p.Journal.Changes,
		Transitions:   p.Journal.Transitions,
		ErrorKey:      p.Journal.ErrorKey, Level: p.Journal.Level,
		Widest:    p.Journal.Widest,
		MovedInto: p.Moved.Into, MovedFrom: p.Moved.From, Marks: p.marks,
	}
}
