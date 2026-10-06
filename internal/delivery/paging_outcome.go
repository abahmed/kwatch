package delivery

// The pipeline marks an incident paged when it hands the message to
// delivery, which is a guess: the pager may refuse the alert for good.
// Delivery corrects it through a PageObserver when a message of the
// incident is given up on at a pager and no pager ever accepted one: the
// alert is not open. A job still queued or retrying says nothing, so the
// guess stands until the job is settled. Only the correction is sent: an
// acceptance that arrives after the incident's resolve was handed over
// must not reopen the flag the resolve closed.

// PageObserver hears that the alert of incident key is not open at the
// pagers although its message was handed to them.
type PageObserver func(key string)

// AttachPageObserver sets the observer. The application attaches it once
// the pipeline exists; it must not block.
func (m *Manager) AttachPageObserver(observe PageObserver) {
	m.pageObserver.Store(&observe)
}

// isPager reports a provider that holds alerts by key.
func isPager(entry *providerEntry) bool {
	return skipsPlainMessages(entry.provider) ||
		receivesPagingOnly(entry.provider)
}

// pagesAlert reports whether the job is one a pager keeps an alert for:
// a message of an incident that is not its resolve.
func (j deliverJob) pagesAlert() bool {
	return j.kind == jobIncident && j.incident != nil &&
		!j.incident.Resolved() && !j.incident.IsInformational()
}

// pageLanded runs when a provider accepted the job.
func (m *Manager) pageLanded(entry *providerEntry, job deliverJob) {
	if !isPager(entry) {
		return
	}
	if m.forgetPagedAfterResolve(job) || !job.pagesAlert() {
		return
	}
	m.pagerLanded.record(job.key(), entry.lookupName())
}

// pageLost runs when the job was given up on at the provider. The alert
// counts as not open only when the lost message was the one that opens
// it and no pager ever accepted a message of the incident. A lost update
// proves nothing: the ledger is in memory, so after a restart it has
// forgotten an alert that is still open.
func (m *Manager) pageLost(entry *providerEntry, job deliverJob) {
	if !isPager(entry) || m.forgetPagedAfterResolve(job) {
		return
	}
	if !job.pagesAlert() || !job.opens() {
		return
	}
	if known, _ := m.pagerLanded.lookup(job.key(), ""); known {
		return
	}
	if observe := m.pageObserver.Load(); observe != nil {
		(*observe)(job.key())
	}
}

// forgetPagedAfterResolve clears what the pagers accepted once the
// incident's resolve was delivered or given up on: the incident is over,
// so a recurrence under the same key must be judged fresh, not by the
// alert of the previous run. It reports whether the job was a resolve.
func (m *Manager) forgetPagedAfterResolve(job deliverJob) bool {
	if job.kind != jobIncident || job.incident == nil ||
		!job.incident.Resolved() {
		return false
	}
	m.pagerLanded.forget(job.key())
	return true
}
