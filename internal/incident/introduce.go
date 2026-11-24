package incident

// Introduce decides the first own message of incident id when a listing
// (a roll-up or a namespace outage) only named it: the listing says that
// it fails, not why, and nothing else may ever change enough to send an
// update that would say so. It reports false for an incident that is
// gone, resolved or quiet, was never carried by a listing, or no longer
// announced. An incident is introduced at most once in its whole life,
// across restarts and reopenings (Marks.Introduced), and never with
// what its last own message already said (Marks.SaidDigest): that would
// repeat the same message as news.
func (m *Manager) Introduce(id string) (Decision, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil || p.quiet || !wasAnnounced(p) || !p.Delivery.RolledUp() ||
		p.marks.Introduced || saidAlready(p) {
		return Decision{}, false
	}
	p.marks.Introduced = true
	return m.decide(p, Update, ReasonIntroduced), true
}

// saidAlready reports an incident whose last own message said what it
// would say now: the same fingerprint.
func saidAlready(p *Incident) bool {
	return p.marks.SaidDigest != "" && p.marks.SaidDigest == fingerprint(p)
}

// Introduced reports an incident whose introduction was decided; it is
// never introduced again.
func (p Incident) Introduced() bool { return p.marks.Introduced }

// NoteSaid records that a message of incident id's own reached chat
// with the fingerprint digest, so an introduction never repeats it
// (saidAlready). It is saved with the record.
func (m *Manager) NoteSaid(id, digest string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && digest != "" {
		p.marks.SaidDigest = digest
	}
}

// AwaitsListing reports an open incident a listing has taken but not
// sent yet: a namespace outage holds its members for a while before
// its message goes out (RecordRolledUp then marks them). Its
// introduction waits for that, within the caller's bound.
func (m *Manager) AwaitsListing(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	return p != nil && !p.quiet && p.State != Resolved &&
		!p.Delivery.RolledUp()
}
