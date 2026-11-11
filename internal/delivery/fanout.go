package delivery

import (
	"fmt"

	"k8s.io/klog/v2"

	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
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
		entry := generation.entries[name]
		if !acceptsPagingOnly(entry, job) {
			// The paging scope of the message excludes this provider,
			// whoever queued the job for it.
			m.outbox.Load().remove(job.outboxID)
			return
		}
		m.offerMarked(entry, name, job)
		return
	}
	var routed []string
	for _, name := range generation.order {
		entry := generation.entries[name]
		if !m.wants(entry, job) || !acceptsPagingOnly(entry, job) {
			continue
		}
		routed = append(routed, name)
		copied := job
		copied.target = name
		copied.outboxID = m.outbox.Load().add(copied, name)
		m.offerMarked(entry, name, copied)
	}
	if len(routed) == 0 {
		result, carrier := unroutedResult(job)
		m.noteUnsent(job, result, carrier)
	}
	m.noteRouted(job, routed...)
}

// offerMarked offers a job to one provider, first recording a lost
// opening the job carries for it.
func (m *Manager) offerMarked(
	entry providerEntry, name string, job deliverJob,
) {
	if job.openLost {
		m.markOpenLost(name, job.key())
	}
	m.offer(entry, job)
}

// unroutedResult is what the send log says of a job no provider took. A
// PagingOnly message is for the paging providers alone: chat heard of it
// through its carrier (the startup summary, a roll-up), or never was
// meant to, so without a paging provider nothing was lost.
func unroutedResult(job deliverJob) (result, carrier string) {
	if job.kind != jobIncident || job.incident == nil ||
		!job.incident.PagingOnly {
		return "not routed", ""
	}
	if c := job.incident.ChatCarrier; c != "" {
		return "carried", c
	}
	return "paging only", ""
}

// acceptsPagingOnly reports whether a provider may receive job. A
// paging-only announcement goes to the providers that skip plain
// messages: the startup summary that stands in for it never reaches them.
// A resolve marked SkipPaging goes to every other provider: the paging
// providers never opened an alert for that incident.
func acceptsPagingOnly(entry providerEntry, job deliverJob) bool {
	if job.kind != jobIncident || job.incident == nil {
		return true
	}
	if job.incident.SkipPaging && skipsPlainMessages(entry.provider) {
		return false
	}
	if !job.incident.PagingOnly {
		return true
	}
	return skipsPlainMessages(entry.provider) ||
		receivesPagingOnly(entry.provider)
}

// receivesPagingOnly reports whether the provider asked for paging-only
// messages although it also takes plain ones.
func receivesPagingOnly(p Provider) bool {
	receiver, ok := p.(deliveryapi.PagingOnlyReceiver)
	return ok && receiver.ReceivesPagingOnly()
}

// offer queues one provider's copy of a job and settles what coalescing
// replaced. The outbox record is written before the job is queued, so a
// worker that delivers it at once always finds a record to remove. The
// caller holds m.mu.
func (m *Manager) offer(entry providerEntry, job deliverJob) {
	defer m.publishQueueDepthLocked(entry)
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
	// A dropped job is counted in the overflow summary, which keeps its
	// outbox record until the summary is sent (recordQueueDrop).
	if result.displaced != nil {
		m.recordQueueDrop(entry, *result.displaced)
	}
	if !result.accepted && (job.announcesPage() || m.replaying) {
		m.keepInBacklogLocked(entry, job)
		return
	}
	if !result.accepted {
		// Nothing queued belongs to the same conversation, so there is
		// nothing to supersede; record the overflow for diagnostics.
		m.recordQueueDrop(entry, job)
	}
}

// keepInBacklogLocked holds a job a full queue could not take in the
// provider's backlog, which streams into the queue as it drains. A page
// announcement belongs here because the overflow summary is no home for
// it: pagers skip summaries, so the alert would never open. So do the
// jobs replayed at Start, which may be more than a queue holds. The
// caller holds m.mu.
func (m *Manager) keepInBacklogLocked(entry providerEntry, job deliverJob) {
	metrics.DefaultRegistry().DeliveryQueueSaturated.Add(1)
	klog.InfoS("queue full; job kept in the backlog",
		"component", "delivery", "operation", "queue_backlog",
		"provider", entry.provider.Name(), "key", job.key())
	job.target = entry.lookupName()
	m.toBacklogLocked(job)
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
	m.pageLost(&entry, job)
	m.addToOverflowSummary(entry.provider.Name(), job)
	logDeadLetter(entry.provider.Name(), job,
		fmt.Errorf("delivery queue saturated"))
}

// opens reports whether the job announces its conversation.
func (j deliverJob) opens() bool {
	return j.kind == jobIncident && j.incident != nil &&
		j.incident.IsOpening()
}
