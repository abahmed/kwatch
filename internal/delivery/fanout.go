package delivery

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/metrics"
)

// fanOut copies a job into the queue of every provider that wants it. The
// caller holds m.mu. A job with a target goes to that provider only.
func (m *Manager) fanOut(job deliverJob) {
	generation := m.currentGenerationLocked()
	if generation == nil {
		return
	}
	job.generation = generation
	if job.queued.IsZero() {
		job.queued = m.nowTime()
	}
	if job.target != "" {
		name, ok := m.resolveTargetLocked(generation, job)
		if !ok {
			return
		}
		job.target = name
		m.offer(generation.entries[name], job)
		return
	}
	for _, name := range generation.order {
		entry := generation.entries[name]
		if !routedTo(entry.routes, job) {
			continue
		}
		copied := job
		copied.target = name
		copied.outboxID = m.outbox.Load().add(copied, name)
		m.offer(entry, copied)
	}
}

// offer queues one provider's copy of a job and settles what coalescing
// replaced. The outbox record is written before the job is queued, so a
// worker that delivers it at once always finds a record to remove. The
// caller holds m.mu.
func (m *Manager) offer(entry providerEntry, job deliverJob) {
	defer m.publishQueueDepthLocked(entry)
	box := m.outbox.Load()
	if old, ok := m.replaceInBacklogLocked(entry, job); ok {
		m.superseded(entry, old, job)
		return
	}
	if m.backlogHoldsConversationLocked(entry, job) {
		m.backlog[entry.lookupName()] = append(
			m.backlog[entry.lookupName()], job)
		m.refillLocked(entry)
		return
	}
	result := offerQueuedJob(entry.ch, job)
	if result.superseded != nil {
		m.superseded(entry, *result.superseded, job)
	}
	if result.displaced != nil {
		box.remove(result.displaced.outboxID)
		m.recordQueueDrop(entry, *result.displaced)
	}
	if !result.accepted {
		// Nothing queued belongs to the same conversation, so there is
		// nothing to supersede; record the overflow for diagnostics.
		box.remove(job.outboxID)
		m.recordQueueDrop(entry, job)
	}
}

// superseded settles a queued job a newer revision replaced before it was
// sent. When the replaced job was the conversation's announcement, the
// provider never received it, which the newer revision must make up for.
func (m *Manager) superseded(
	entry providerEntry, old, replacement deliverJob,
) {
	m.outbox.Load().remove(old.outboxID)
	if old.opens() && !replacement.opens() {
		m.markOpenLost(entry.lookupName(), old.key())
	}
}

func (m *Manager) recordQueueDrop(
	entry providerEntry,
	job deliverJob,
) {
	metrics.DefaultRegistry().NotificationsDropped.Add(1)
	metrics.DefaultRegistry().DeliveryQueueSaturated.Add(1)
	m.openFailed(&entry, job)
	m.addToOverflowSummary(entry.provider.Name(), job)
	m.recordDeadLetter(entry.provider.Name(), job,
		fmt.Errorf("delivery queue saturated"))
}

// opens reports whether the job announces its conversation.
func (j deliverJob) opens() bool {
	return j.kind == jobIncident && j.incident != nil &&
		j.incident.IsOpening()
}
