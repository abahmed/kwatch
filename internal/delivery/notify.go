package delivery

import (
	"errors"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// Notify queues a plain message for every provider.
//
// It never sends on the caller's goroutine: the startup banner is sent
// from the goroutine that then starts the informers, so an inline send to
// an unreachable provider would hold up monitoring. Queuing also puts
// messages on the same paced, summarized, dead-lettered path as
// incidents, so a message that fails everywhere appears in /deadletters.
func (m *Manager) Notify(msg string) {
	klog.InfoS("sending message", "messageLength", len(msg))
	m.enqueue(deliverJob{kind: jobMessage, msg: msg})
}

// enqueue retains jobs until workers exist. It never performs provider I/O on
// the caller's goroutine before Start.
func (m *Manager) enqueue(job deliverJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.state == stateReconfigureDraining:
		// The next generation replays pending jobs.
		m.holdPendingLocked(job)
	case m.state == stateStopped:
		recordStoppedDrop()
	case m.state.accepting():
		m.fanOut(job)
	default:
		m.holdPendingLocked(job)
	}
}

// Pending-queue losses, as dead-letter reasons.
var (
	errPendingFull        = errors.New("pending_queue_full")
	errStoppedBeforeStart = errors.New("stopped_before_start")
)

// holdPendingLocked keeps a job until Start. The pending queue coalesces
// like a provider queue: a newer message of a conversation replaces the
// waiting one, and a resolve is never the job that is dropped. The caller
// holds m.mu.
//
// Before the first Start the job is split into one copy per provider and
// each copy gets an outbox record, so a crash before delivery starts does
// not lose it. A job held during a reconfiguration stays whole: it goes
// to the providers of the next generation.
func (m *Manager) holdPendingLocked(job deliverJob) {
	if job.queued.IsZero() {
		job.queued = m.nowTime()
	}
	generation := m.currentGenerationLocked()
	if m.state.stopped() || job.target != "" || generation == nil ||
		len(generation.order) == 0 {
		m.holdOnePendingLocked(job, channelCap)
		return
	}
	limit := channelCap * len(generation.order)
	for _, name := range generation.order {
		if !routedTo(generation.entries[name].routes, job) {
			continue
		}
		copied := job
		copied.target = name
		copied.outboxID = m.outbox.Load().add(copied, name)
		m.holdOnePendingLocked(copied, limit)
	}
}

func (m *Manager) holdOnePendingLocked(job deliverJob, limit int) {
	queued, result := coalesce(m.pending, job, limit)
	m.pending = queued
	box := m.outbox.Load()
	if result.superseded != nil {
		box.remove(result.superseded.outboxID)
	}
	if result.displaced != nil {
		box.remove(result.displaced.outboxID)
		recordPendingDrop(*result.displaced, errPendingFull)
		m.recordDeadLetter(pendingProvider, *result.displaced, errPendingFull)
	}
	if !result.accepted {
		box.remove(job.outboxID)
		recordPendingDrop(job, errPendingFull)
		m.recordDeadLetter(pendingProvider, job, errPendingFull)
	}
}

func recordStoppedDrop() {
	metrics.DefaultRegistry().NotificationsDropped.Add(1)
	klog.V(2).InfoS("notification dropped; delivery is not accepting jobs")
}

// recordPendingDrop counts a job lost before any worker could take it:
// the pending queue was full, or delivery stopped before it started.
func recordPendingDrop(job deliverJob, reason error) {
	registry := metrics.DefaultRegistry()
	registry.NotificationsDropped.Add(1)
	registry.DeliveryPendingDropped.Add(1)
	klog.V(2).InfoS("pending notification dropped",
		"component", "delivery", "operation", "pending_drop",
		"key", job.key(), "reason", reason.Error())
}

// dropPendingLocked settles jobs that waited for a Start that will not
// come. A job with an outbox record (a restored one, or one held after
// the outbox was attached) is deferred to the next session; the rest are
// counted and dead-lettered. The caller holds m.mu.
func (m *Manager) dropPendingLocked() {
	for _, job := range m.pending {
		if job.outboxID != "" && m.outbox.Load() != nil {
			m.deferJobLocked(job.target, job, errStoppedBeforeStart)
			continue
		}
		recordPendingDrop(job, errStoppedBeforeStart)
		m.recordDeadLetter(pendingProvider, job, errStoppedBeforeStart)
	}
	m.pending = nil
	for _, job := range m.restored {
		m.deferJobLocked(job.target, job, errStoppedBeforeStart)
	}
	m.restored = nil
}
