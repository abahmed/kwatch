package delivery

import (
	"context"
	"errors"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// workOne paces and delivers one job for a worker. It returns false when
// the worker's context ended first; the job then goes to Stop's drain
// instead of being dead-lettered, because it left the queue Stop drains.
//
// A provider that fails in a way that passes keeps the job: the worker
// waits out the provider's backoff and sends the newest revision of the
// conversation once the provider is back. Only a rejection, or a job
// older than outboxMaxAge, is given up.
func (m *Manager) workOne(
	ctx context.Context, entry *providerEntry, job deliverJob,
) bool {
	defer m.markBusy()()
	// Routing is decided before pacing. A job this provider does not want
	// is not a delivery, and making it wait its turn spent the provider's
	// send slot on nothing: with a route that matches one namespace, a
	// storm elsewhere throttled the alerts that did match.
	if !routedTo(entry.routes, job) {
		m.outbox.Load().remove(job.outboxID)
		return true
	}
	admitted := false
	for {
		job = m.newestRevision(entry, job)
		if m.expired(job) {
			m.openFailed(entry, job)
			m.recordTerminalFailure(entry, job, errJobExpired)
			return true
		}
		if !admitted && m.foldOverBudget(entry, job) {
			return true
		}
		admitted = true
		// Pace before delivering: the queue absorbs the burst, the
		// provider sees a rate it tolerates, and a blocked provider is
		// waited out here. Cancellation stops delivery promptly.
		if ctx.Err() != nil ||
			!m.waitForSendSlot(ctx, entry.provider.Name()) {
			m.handOff(entry, job)
			return false
		}
		job = m.newestRevision(entry, job)
		switch m.deliver(ctx, entry, job) {
		case outcomeInterrupted:
			m.handOff(entry, job)
			return false
		case outcomeRetryLater:
			metrics.DefaultRegistry().DeliveryRetries.Add(1)
			continue
		}
		return true
	}
}

// expired reports whether a job is too old to be worth sending.
func (m *Manager) expired(job deliverJob) bool {
	return !job.queued.IsZero() &&
		m.nowTime().Sub(job.queued) > outboxMaxAge
}

// handOff keeps a job a cancelled worker could not finish, for Stop.
func (m *Manager) handOff(entry *providerEntry, job deliverJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handoff == nil {
		m.handoff = make(map[string][]deliverJob)
	}
	name := entry.lookupName()
	m.handoff[name] = append(m.handoff[name], job)
}

// takeHandoff returns and forgets the jobs handed off for a provider.
func (m *Manager) takeHandoff(entry providerEntry) []deliverJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := entry.lookupName()
	jobs := m.handoff[name]
	delete(m.handoff, name)
	return jobs
}

// deferJob settles a job delivery could not send before it stopped. A
// reconfiguration keeps it for the next generation. A job with an outbox
// record is sent by the next session, so it is counted as deferred, not
// as a dead letter; only a job nothing will resend is dead-lettered.
func (m *Manager) deferJob(entry *providerEntry, job deliverJob, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deferJobLocked(entry.lookupName(), job, err)
}

// deferJobLocked is deferJob for a caller that holds m.mu.
func (m *Manager) deferJobLocked(provider string, job deliverJob, err error) {
	if m.state.reconfiguring() && job.target != "" {
		m.toBacklogLocked(job)
		return
	}
	if job.outboxID != "" && m.outbox.Load() != nil {
		metrics.DefaultRegistry().Delivery.Deferred.Add(1)
		klog.V(2).InfoS("delivery deferred to the next session",
			"component", "delivery", "operation", "defer",
			"provider", provider, "key", job.key(),
			"reason", deadLetterReason(err))
		return
	}
	if errors.Is(err, errProviderUnavailable) {
		metrics.DefaultRegistry().NotificationsDropped.Add(1)
	}
	m.recordDeadLetter(provider, job, err)
}
