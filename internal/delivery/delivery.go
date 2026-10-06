package delivery

import (
	"context"
	"errors"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

// deliverJob is one queued delivery. A job is an incident or a plain
// message; queueing both means messages are paced, summarized on overflow
// and dead-lettered exactly like incidents instead of being written straight
// to the provider from whatever goroutine happened to call Notify.
type deliverJob struct {
	generation *providerGeneration
	kind       jobKind
	msg        string
	incident   *notification.Message
	// target is the lookup name of the one provider the job is for. A new
	// job has none until fanOut copies it into each provider queue; a job
	// restored from the outbox keeps the provider it was saved for.
	target string
	// outboxID names the job's outbox record; "" when none was written.
	outboxID string
	// queued is when the job was first queued; zero when unknown. A job
	// older than outboxMaxAge is given up instead of being sent.
	queued time.Time
}

// isResolve reports whether the job closes an incident.
func (j deliverJob) isResolve() bool {
	return j.kind == jobIncident && j.incident != nil &&
		j.incident.Resolved()
}

// key names the job in logs and dead letters.
func (j deliverJob) key() string {
	if j.incident != nil {
		return j.incident.Key
	}
	return "message"
}

// deliverFallback re-sends a job through a provider's configured fallback.
// The job shape lives in the job, so one call covers every shape; only the
// retry budget and the "primary failed" prefix differ from a normal
// delivery.
func (m *Manager) deliverFallback(
	ctx context.Context,
	entry *providerEntry,
	primary string,
	job deliverJob,
) error {
	if !m.wants(*entry, job) || !acceptsPagingOnly(*entry, job) ||
		(job.informational() && skipsPlainMessages(entry.provider)) {
		return errFallbackNotRouted
	}
	opts := deliverOpts{retry: fallbackRetryConfig(entry.retry)}
	if entry.provider.Name() != primary {
		opts.fallbackFrom = primary
	}
	return m.dispatch(ctx, entry, job, opts)
}

// errFallbackNotRouted means the fallback's routes, or the paging rules of
// the job (PagingOnly, SkipPaging), exclude it, or the job only informs
// and the fallback skips such messages, so the job was not delivered
// anywhere.
var errFallbackNotRouted = errors.New("fallback_not_routed")

// channelCap is the capacity of each provider queue and, per provider, of
// the pending queue that holds jobs created before Start.
const channelCap = 256

// defaultMaxBackoff caps retry backoff when the configured cap is negative.
const defaultMaxBackoff = 30 * time.Second

func fallbackRetryConfig(rc retryConfig) retryConfig {
	return normalizeRetryConfig(rc)
}

// recordDeadLetter counts a job that was given up on. provider names the
// provider that failed last, or pendingProvider for a job lost before any
// provider queue took it. Provider and transport errors stay in the
// structured logs of the failing send; the dead-letter log carries only a
// bounded reason code.
func (m *Manager) recordDeadLetter(
	provider string,
	job deliverJob,
	err error,
) {
	metrics.DefaultRegistry().DeliveryDeadLetters.Add(1)
	noteLostResolve(job, deadLetterReason(err))
	klog.InfoS("delivery dead-lettered",
		"component", "delivery", "operation", "dead_letter",
		"provider", provider, "key", job.key(),
		"reason", deadLetterReason(err))
}

// noteLostResolve makes the loss of a resolve visible: its alert may now
// stay open until someone closes it by hand. Every path that gives up on
// a job (a permanent failure, expiry, a full queue) comes through the
// dead letter.
func noteLostResolve(job deliverJob, reason string) {
	if !job.isResolve() {
		return
	}
	metrics.DefaultRegistry().DeliveryResolvesLost.Add(1)
	klog.ErrorS(nil, "resolve was not delivered; its alert may stay open",
		"component", "delivery", "key", job.key(),
		"alertKey", job.incident.DedupKey, "reason", reason)
}

// pendingProvider labels dead letters of jobs that never reached a
// provider queue.
const pendingProvider = "pending"

func deadLetterReason(err error) string {
	if errors.Is(err, errDeliveryShutdown) {
		return "delivery_shutdown"
	}
	if errors.Is(err, errFallbackNotRouted) {
		return "fallback_not_routed"
	}
	if errors.Is(err, errPendingFull) {
		return "pending_queue_full"
	}
	if errors.Is(err, errStoppedBeforeStart) {
		return "stopped_before_start"
	}
	if errors.Is(err, errJobExpired) {
		return "expired"
	}
	if err != nil && strings.Contains(
		strings.ToLower(err.Error()), "queue saturated",
	) {
		return "queue_saturated"
	}
	return "delivery_failed"
}

// deliverOutcome is how one delivery attempt ended.
type deliverOutcome int

const (
	// outcomeDelivered: a provider (or its fallback) accepted the job,
	// or the provider's routes do not want it.
	outcomeDelivered deliverOutcome = iota
	// outcomeFailed: the provider rejected the job for good and nothing
	// else accepted it; it was dead-lettered.
	outcomeFailed
	// outcomeInterrupted: ctx ended before any provider accepted the job.
	// It was not dead-lettered; the caller decides what happens to it.
	outcomeInterrupted
	// outcomeRetryLater: the provider failed in a way that passes (an
	// outage, a timeout, a rate limit). The job keeps its outbox record
	// and is sent again once the provider's backoff ends.
	outcomeRetryLater
)

// deliverOne sends one job once, for the shutdown drain. A job it cannot
// send now is deferred: kept for the next session or generation when it
// has an outbox record, dead-lettered otherwise.
func (m *Manager) deliverOne(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
) bool {
	switch m.deliver(ctx, entry, job) {
	case outcomeDelivered:
		return true
	case outcomeInterrupted:
		m.deferJob(entry, job, errDeliveryShutdown)
	case outcomeRetryLater:
		m.deferJob(entry, job, errProviderUnavailable)
	}
	return false
}

// errProviderUnavailable marks a job whose provider kept failing until
// the job could not wait any longer.
var errProviderUnavailable = errors.New("provider_unavailable")

// errJobExpired marks a job older than outboxMaxAge.
var errJobExpired = errors.New("expired")

// deliverViaFallback runs after the primary provider rejected the job
// with err. A job is dead-lettered and counted as dropped once, when
// nothing is left to try: here when there is no fallback, or when the
// fallback fails too. A job the fallback delivers was not dropped.
func (m *Manager) deliverViaFallback(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	err error,
) deliverOutcome {
	p := entry.provider
	fallback, ok := m.fallbackFor(entry.fallbackName, job.generation)
	if !ok {
		m.recordTerminalFailure(entry, job, err)
		return outcomeFailed
	}
	fbErr := m.deliverFallback(ctx, &fallback, p.Name(), job)
	if fbErr == nil {
		m.accepted(job)
		return outcomeDelivered
	}
	if ctx.Err() != nil {
		return outcomeInterrupted
	}
	klog.ErrorS(loggedErr(fbErr), "fallback delivery failed",
		"provider", fallback.provider.Name())
	m.recordTerminalFailure(&fallback, job, fbErr)
	return outcomeFailed
}

// fallbackOrRetry runs after the primary failed in a way that passes.
// The fallback, when there is one, gets the job now; otherwise, or when
// the fallback fails too, the job waits for the primary to recover.
func (m *Manager) fallbackOrRetry(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
) deliverOutcome {
	fallback, ok := m.fallbackFor(entry.fallbackName, job.generation)
	if !ok || mustReachPrimary(entry, job) {
		return outcomeRetryLater
	}
	err := m.deliverFallback(ctx, &fallback, entry.provider.Name(), job)
	switch {
	case err == nil:
		m.accepted(job)
		return outcomeDelivered
	case ctx.Err() != nil:
		return outcomeInterrupted
	}
	klog.ErrorS(loggedErr(err), "fallback delivery failed",
		"provider", fallback.provider.Name())
	return outcomeRetryLater
}

// mustReachPrimary reports whether only the primary can do the job. A
// pager or issue tracker opens and closes alerts by key; a fallback chat
// message about the same incident does neither, and chat already gets
// its own copy of the message. The primary is retried until it takes it.
func mustReachPrimary(entry *providerEntry, job deliverJob) bool {
	return skipsPlainMessages(entry.provider) &&
		(job.isResolve() || job.opens())
}

// informational reports whether the job only informs: a plain message
// or a summary. Nothing closes the alert such a message would open.
func (j deliverJob) informational() bool {
	if j.kind == jobMessage {
		return true
	}
	return j.incident != nil && j.incident.IsInformational()
}

// deliver sends one job and settles its outbox record, except when ctx
// ended first or the provider asked to be retried later: such a job
// keeps its record and is not dead-lettered.
func (m *Manager) deliver(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
) deliverOutcome {
	if job.generation == nil {
		m.mu.Lock()
		job.generation = cloneProviderGeneration(
			m.currentGenerationLocked(), false,
		)
		m.mu.Unlock()
	}
	p := entry.provider
	metrics.DefaultRegistry().NotificationsTotal.Add(1)

	// Routes are evaluated before rendering: a filtered incident should not
	// pay for message building, and routes depend only on the incident.
	if !m.wants(*entry, job) {
		klog.V(4).InfoS("incident filtered by route",
			"provider", p.Name(),
			"key", job.key())
		m.outbox.Load().remove(job.outboxID)
		return outcomeDelivered
	}

	warnUnmappedResolve(entry, job)
	err := m.dispatch(ctx, entry, job, deliverOpts{retry: entry.retry})
	if err == nil {
		m.recordProviderSuccess(entry)
		m.openSettled(entry, job)
		m.pageLanded(entry, job)
		m.accepted(job)
		return outcomeDelivered
	}
	if ctx.Err() != nil {
		// Shutting down: no fallback, and no permanent dead letter.
		return outcomeInterrupted
	}
	klog.ErrorS(loggedErr(err), "failed to send",
		"provider", p.Name(), "key", job.key())
	m.recordProviderFailure(entry, err)
	switch {
	case isRateLimited(err):
		// A rate limit is not an outage: the fallback is not flooded
		// with everything the primary asked to slow down.
		return outcomeRetryLater
	case transport.IsPermanent(err):
		m.openFailed(entry, job)
		return m.deliverViaFallback(ctx, entry, job, err)
	}
	return m.fallbackOrRetry(ctx, entry, job)
}

// accepted settles a job a provider took: its outbox record is removed.
func (m *Manager) accepted(job deliverJob) {
	m.outbox.Load().remove(job.outboxID)
	m.signalDelivered()
}

// recordTerminalFailure counts a job that no provider accepted,
// dead-letters it against the provider that failed last, and removes its
// outbox record: sending it again would fail the same way.
func (m *Manager) recordTerminalFailure(
	entry *providerEntry, job deliverJob, err error,
) {
	registry := metrics.DefaultRegistry()
	registry.NotificationsDropped.Add(1)
	registry.DeliveryTerminalErrors.Add(1)
	m.recordDeadLetter(entry.provider.Name(), job, err)
	m.pageLost(entry, job)
	m.outbox.Load().remove(job.outboxID)
}

func (m *Manager) signalDelivered() {
	if m.onDelivered != nil {
		m.onDelivered()
	}
}

// fallbackFor returns a value snapshot of the configured fallback entry.
// Resolving by name keeps fallback state independent of generation storage.
func (m *Manager) fallbackFor(
	name string,
	generation *providerGeneration,
) (providerEntry, bool) {
	if name == "" {
		return providerEntry{}, false
	}
	if generation != nil {
		fallback, ok := generation.entries[strings.ToLower(name)]
		return fallback, ok
	}
	return providerEntry{}, false
}
