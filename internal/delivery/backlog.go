package delivery

import (
	"strings"

	"k8s.io/klog/v2"

	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

// A restart can leave more unsent jobs in the outbox than a provider
// queue holds. Restored jobs therefore wait in a per-provider backlog and
// stream into the queue as its worker drains it, instead of overflowing
// it. The backlog also keeps the jobs a reconfiguration could not send,
// for the next generation.

// backlogReserve is the queue room restored jobs leave free for new ones.
const backlogReserve = channelCap / 4

// toBacklogLocked adds jobs for their target provider's backlog, keeping
// only the newest revision of each conversation. The caller holds m.mu.
func (m *Manager) toBacklogLocked(job deliverJob) {
	if m.backlog == nil {
		m.backlog = make(map[string][]deliverJob)
	}
	queued := m.backlog[job.target]
	if index := replacementIndex(queued, job); index >= 0 {
		m.outbox.Load().remove(queued[index].outboxID)
		queued[index] = job
		return
	}
	m.backlog[job.target] = append(queued, job)
}

// replaceInBacklogLocked lets a new revision replace a waiting restored
// one of the same conversation, so the older one is never sent after it.
func (m *Manager) replaceInBacklogLocked(
	entry providerEntry, job deliverJob,
) (deliverJob, bool) {
	queued := m.backlog[entry.lookupName()]
	index := replacementIndex(queued, job)
	if index < 0 {
		return deliverJob{}, false
	}
	old := queued[index]
	queued[index] = job
	return old, true
}

// backlogHoldsConversationLocked reports whether the provider's backlog
// has an incident job that must be sent before the arriving one: the same
// conversation, or the same paging identity (a recurrence has a new key
// but shares its DedupKey). Such a job is appended behind the backlog so
// a waiting resolve is never overtaken. The caller holds m.mu.
func (m *Manager) backlogHoldsConversationLocked(
	entry providerEntry, job deliverJob,
) bool {
	if job.kind != jobIncident || job.incident == nil {
		return false
	}
	for _, q := range m.backlog[entry.lookupName()] {
		if q.kind != jobIncident || q.incident == nil {
			continue
		}
		if q.incident.Key == job.incident.Key ||
			(job.incident.DedupKey != "" &&
				q.incident.DedupKey == job.incident.DedupKey) {
			return true
		}
	}
	return false
}

// refillLocked moves backlog jobs into the provider queue while it has
// room beyond the reserve. The caller holds m.mu and the queue is open.
func (m *Manager) refillLocked(entry providerEntry) {
	name := entry.lookupName()
	queued := m.backlog[name]
	for len(queued) > 0 && entry.ch != nil &&
		len(entry.ch) < cap(entry.ch)-backlogReserve {
		entry.ch <- queued[0]
		queued = queued[1:]
	}
	if len(queued) == 0 {
		delete(m.backlog, name)
	} else {
		m.backlog[name] = queued
	}
	m.publishQueueDepthLocked(entry)
}

// refillFromBacklog is refillLocked for a worker between jobs.
func (m *Manager) refillFromBacklog(entry providerEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.accepting() {
		m.refillLocked(entry)
	}
}

// takeBacklog returns and forgets a provider's backlog.
func (m *Manager) takeBacklog(entry providerEntry) []deliverJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := entry.lookupName()
	jobs := m.backlog[name]
	delete(m.backlog, name)
	return jobs
}

// queuedFor counts the jobs waiting for a provider.
func (m *Manager) queuedFor(entry providerEntry) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	depth := len(m.backlog[entry.lookupName()])
	if entry.ch != nil {
		depth += len(entry.ch)
	}
	return depth
}

// resolveTargetLocked finds the provider a targeted job goes to. A job
// saved for a provider that is no longer configured goes to the one
// configured provider of the same type, such as after a rename between
// "incident.io" and "incidentio"; otherwise it is dropped and counted.
func (m *Manager) resolveTargetLocked(
	generation *providerGeneration, job deliverJob,
) (string, bool) {
	if _, ok := generation.entries[job.target]; ok {
		return job.target, true
	}
	var matches []string
	want := providerType(job.target)
	for _, name := range generation.order {
		if providerType(name) == want {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		klog.InfoS("queued notification re-targeted to renamed provider",
			"component", "delivery", "operation", "retarget",
			"from", job.target, "to", matches[0])
		return matches[0], true
	}
	m.outbox.Load().remove(job.outboxID)
	recordOutboxDrop(job.outboxID, "provider_removed")
	klog.InfoS("queued notification dropped: its provider is gone",
		"component", "delivery", "operation", "retarget",
		"provider", job.target, "candidates", len(matches))
	return "", false
}

// providerType is the canonical catalog identity of a provider name.
func providerType(name string) string {
	name = strings.ToLower(name)
	if canonical, ok := providercatalog.Aliases()[name]; ok {
		return canonical
	}
	return name
}

// newestRevision lets a newer queued revision of the same conversation
// take the place of the job a worker is about to send. During an outage
// the worker holds one job while newer revisions queue behind it, where
// they replace each other; after recovery only the newest is sent, and
// a resolve is never replaced by anything but another resolve.
func (m *Manager) newestRevision(
	entry *providerEntry, job deliverJob,
) deliverJob {
	if job.kind != jobIncident || job.incident == nil || entry.ch == nil {
		return job
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.state.accepting() {
		return job
	}
	queued := drainQueuedJobs(entry.ch)
	index := newerRevisionIndex(queued, job)
	if index >= 0 {
		newer := queued[index]
		queued = append(queued[:index], queued[index+1:]...)
		m.superseded(*entry, job, newer)
		job = newer
	}
	refillQueuedJobs(entry.ch, queued)
	m.publishQueueDepthLocked(*entry)
	return job
}

// newerRevisionIndex finds the queued job that may replace held: the
// next one of the same conversation. Coalescing leaves at most a resolve
// followed by a reopening queued per conversation, so taking the next
// one, never a later one, keeps the conversation's order.
func newerRevisionIndex(queued []deliverJob, held deliverJob) int {
	for i, q := range queued {
		if q.kind != jobIncident || q.incident == nil ||
			q.incident.Key != held.incident.Key || q.target != held.target {
			continue
		}
		if held.isResolve() && !q.isResolve() {
			return -1
		}
		return i
	}
	return -1
}
