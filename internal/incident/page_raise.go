package incident

// A pager that keeps the urgency an alert was opened with never pages
// for an incident it first heard below the page tier. The pipeline
// reports each message the pagers hear here, and the first one at the
// page tier after a lower one raises the page: it opens a second alert
// under its own key (notification.Message.RaisesPage). Every later
// message goes to that key (PageKeyed), and the resolve closes both. The
// marks are saved, so a restart keeps the second key; an incident born
// at the page tier never gets them.

// RecordPagerTier remembers that the paging providers heard a message of
// incident id at the page tier (atPage) or below it. It reports whether
// this message raises the page and whether it goes to the page key.
func (m *Manager) RecordPagerTier(
	id string, atPage bool,
) (raises, keyed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil {
		return false, false
	}
	marks := &p.marks
	switch {
	case marks.PageRaised:
		return false, true
	case !atPage:
		marks.PagedBelowPage = true
		return false, false
	case marks.PagedBelowPage:
		marks.PageRaised = true
		return true, true
	}
	return false, false
}

// ClosePageKeys forgets the page raise of incident id as it resolves, so
// a reopen starts fresh. It reports whether the resolve must also close
// the page key.
func (m *Manager) ClosePageKeys(id string) (keyed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil {
		return false
	}
	keyed = p.marks.PageRaised
	p.marks.PagedBelowPage, p.marks.PageRaised = false, false
	return keyed
}
