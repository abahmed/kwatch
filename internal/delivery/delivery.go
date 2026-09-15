package delivery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

// deliverJob is one queued delivery. A job is an incident, a plain message,
// or a legacy event; queueing all three means messages are paced, digested
// and dead-lettered exactly like incidents instead of being written straight
// to the provider from whatever goroutine happened to call Notify.
type deliverJob struct {
	generation *providerGeneration
	kind       jobKind
	inc        *model.Incident
	action     model.IncidentAction
	insight    *insight.Insight
	msg        string
	ev         *event.Event
}

// key names the job in logs and dead letters.
func (j deliverJob) key() string {
	if j.inc != nil {
		return string(j.inc.Key)
	}
	if j.ev != nil {
		return j.ev.Reason
	}
	return "message"
}

// DeadLetterEntry is kept as a delivery alias for callers that inspect the
// delivery package. The diagnostic wire type belongs to model.
type DeadLetterEntry = model.DeadLetterEntry

// deliverFallback re-sends a job through a provider's configured fallback.
//
// A fallback used to have three entry points -- one per job shape -- each
// repeating the provider-type switch. It is one call now: the shape lives in
// the job, and only the retry budget and the "primary failed" prefix differ
// from a normal delivery.
func (a *Manager) deliverFallback(
	ctx context.Context,
	entry *providerEntry,
	primary string,
	job deliverJob,
) error {
	if job.kind == jobIncident && !shouldDeliver(entry.routes, job.inc) {
		return nil
	}
	opts := deliverOpts{retry: fallbackRetryConfig(entry.retry)}
	if entry.provider.Name() != primary {
		opts.prefix = "[fallback — primary " + primary + " failed] "
	}
	return a.dispatch(ctx, entry, job, opts)
}

const channelCap = 256
const dlqCap = 100

const defaultMaxBackoff = 30 * time.Second

func fallbackRetryConfig(rc retryConfig) retryConfig {
	return normalizeRetryConfig(rc)
}

func (a *Manager) recordDeadLetter(
	entry *providerEntry,
	job deliverJob,
	err error,
) {
	metrics.DefaultRegistry().DeliveryDeadLetters.Add(1)
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	a.dlqRing[a.dlqHead] = DeadLetterEntry{
		Provider:  entry.provider.Name(),
		Key:       job.key(),
		Action:    job.action,
		Error:     err.Error(),
		Timestamp: a.nowTime(),
	}
	a.dlqHead = (a.dlqHead + 1) % dlqCap
	if a.dlqCount < dlqCap {
		a.dlqCount++
	}
}

// deliverOne handles the full send, retry, dead-letter and fallback for one
// job on one provider.
func (a *Manager) deliverOne(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
) bool {
	return a.deliverOneWithContext(ctx, entry, job)
}

func (a *Manager) deliverOneWithContext(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
) bool {
	p := entry.provider
	metrics.DefaultRegistry().NotificationsTotal.Add(1)

	// Routes are evaluated before rendering: a filtered incident should not
	// pay for message building, and routes depend only on the incident.
	if job.kind == jobIncident && !shouldDeliver(entry.routes, job.inc) {
		klog.V(4).InfoS("incident filtered by route",
			"provider", p.Name(),
			"key", job.key())
		return true
	}

	err := a.dispatch(ctx, entry, job, deliverOpts{retry: entry.retry})
	if err == nil {
		return true
	}
	metrics.DefaultRegistry().NotificationsDropped.Add(1)
	klog.ErrorS(err, "failed to send",
		"provider", p.Name(), "key", job.key())
	a.recordDeadLetter(entry, job, err)
	fallback, ok := a.fallbackFor(
		entry.fallbackName, job.generation,
	)
	if !ok {
		metrics.DefaultRegistry().DeliveryTerminalErrors.Add(1)
		return false
	}
	if fbErr := a.deliverFallback(
		ctx, &fallback, p.Name(), job,
	); fbErr != nil {
		metrics.DefaultRegistry().DeliveryTerminalErrors.Add(1)
		klog.ErrorS(fbErr, "fallback delivery failed",
			"provider", fallback.provider.Name())
		a.recordDeadLetter(&fallback, job, fbErr)
		return false
	}
	return true
}

// fallbackFor returns a value snapshot of the configured fallback entry.
// Resolving by name keeps fallback state independent of generation storage.
func (a *Manager) fallbackFor(
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
	a.mu.Lock()
	defer a.mu.Unlock()
	if generation := a.currentGenerationLocked(); generation != nil {
		fallback, ok := generation.entries[strings.ToLower(name)]
		return fallback, ok
	}
	return providerEntry{}, false
}

// buildMessage produces a formatted message string for the given incident.
// Uses the context-adaptive ReportBuilder and PlainTextRenderer.

func (a *Manager) fanOut(job deliverJob) {
	generation := a.currentGenerationLocked()
	if generation == nil {
		return
	}
	job.generation = generation
	for _, name := range generation.order {
		entry := generation.entries[name]
		select {
		case entry.ch <- job:
		default:
			// Saturated. Drop the arriving job rather than evicting a queued
			// one.
			//
			// During a storm the earliest notifications are the ones worth
			// keeping: they are the root cause, and everything after tends to
			// be downstream symptoms of it. Evicting the oldest also risks
			// discarding an incident's CREATE while keeping a later UPDATE for
			// it, which reaches the channel as an edit to something that was
			// never announced.
			//
			// Nothing is lost silently — the dropped job goes to the
			// dead-letter queue, which is readable over the health endpoint.
			metrics.DefaultRegistry().NotificationsDropped.Add(1)
			metrics.DefaultRegistry().DeliveryQueueSaturated.Add(1)
			a.digestAdd(entry.provider.Name(), job)
			a.recordDeadLetter(
				&entry,
				job,
				fmt.Errorf("delivery queue saturated"),
			)
		}
	}
}

// deliverAllSync sends directly to every provider, bypassing the queue. It
// is only for callers that run before Start -- kwatch replay -- where there
// are no provider workers to pick a job up.
func (a *Manager) deliverAllSync(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) {
	job := incidentJob(inc, action, ins)
	a.mu.Lock()
	generation := cloneProviderGeneration(
		a.currentGenerationLocked(), false,
	)
	a.mu.Unlock()
	if generation == nil {
		return
	}
	job.generation = generation
	for _, name := range generation.order {
		entry := generation.entries[name]
		if !shouldDeliver(entry.routes, inc) {
			continue
		}
		a.deliverOne(context.Background(), &entry, job)
	}
}
