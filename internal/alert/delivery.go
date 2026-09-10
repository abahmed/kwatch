package alert

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
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
	kind    jobKind
	inc     *model.Incident
	action  model.IncidentAction
	insight *insight.Insight
	msg     string
	ev      *event.Event
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

type DeadLetterEntry struct {
	Provider  string               `json:"provider"`
	Key       string               `json:"key"`
	Action    model.IncidentAction `json:"action"`
	Error     string               `json:"error"`
	Timestamp time.Time            `json:"timestamp"`
}

// deliverFallback re-sends a job through a provider's configured fallback.
//
// A fallback used to have three entry points -- one per job shape -- each
// repeating the provider-type switch. It is one call now: the shape lives in
// the job, and only the retry budget and the "primary failed" prefix differ
// from a normal delivery.
func (a *AlertManager) deliverFallback(
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
	// Already inside this provider's delivery context: re-entering would
	// borrow a context from itself.
	if entry.provider.Name() == primary {
		return a.dispatch(ctx, entry, job, opts)
	}
	var err error
	util.WithProviderContext(entry.provider.Name(), ctx, func() {
		err = a.dispatch(ctx, entry, job, opts)
	})
	return err
}

const channelCap = 256
const dlqCap = 100

const defaultMaxBackoff = 30 * time.Second

func fallbackRetryConfig(rc retryConfig) retryConfig {
	return normalizeRetryConfig(rc)
}

func (a *AlertManager) recordDeadLetter(
	entry *providerEntry,
	job deliverJob,
	err error,
) {
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
}

// deliverOne handles the full send, retry, dead-letter and fallback for one
// job on one provider.
func (a *AlertManager) deliverOne(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
) bool {
	delivered := false
	util.WithProviderContext(entry.provider.Name(), ctx, func() {
		delivered = a.deliverOneWithContext(ctx, entry, job)
	})
	return delivered
}

func (a *AlertManager) deliverOneWithContext(
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
	if entry.fallback == nil {
		return false
	}
	if fbErr := a.deliverFallback(
		ctx, entry.fallback, p.Name(), job,
	); fbErr != nil {
		klog.ErrorS(fbErr, "fallback delivery failed",
			"provider", entry.fallback.provider.Name())
		a.recordDeadLetter(entry.fallback, job, fbErr)
		return false
	}
	return true
}

// buildMessage produces a formatted message string for the given incident.
// Uses the context-adaptive ReportBuilder and PlainTextRenderer.

func (a *AlertManager) fanOut(job deliverJob) {
	for _, entry := range a.entries {
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
func (a *AlertManager) deliverAllSync(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) {
	job := incidentJob(inc, action, ins)
	for i := range a.entries {
		entry := &a.entries[i]
		if !shouldDeliver(entry.routes, inc) {
			continue
		}
		a.deliverOne(context.Background(), entry, job)
	}
}
