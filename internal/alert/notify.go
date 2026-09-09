package alert

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// Notify queues a plain message for every provider.
//
// It used to send inline, on the caller's goroutine, with per-provider
// retries and no dead-letter record. The startup banner is sent from the
// goroutine that then starts the informers, so one unreachable provider held
// up monitoring behind three backoffs; and a message that failed everywhere
// vanished without appearing in /deadletters. Queuing puts messages on the
// same paced, digested, dead-lettered path as incidents.
func (a *AlertManager) Notify(msg string) {
	klog.InfoS("sending message", "msg", msg)
	a.enqueue(deliverJob{kind: jobMessage, msg: msg})
}

// NotifyEvent queues a legacy event for every provider.
func (a *AlertManager) NotifyEvent(ev event.Event) {
	klog.InfoS(
		"sending event",
		"resource", ev.Resource,
		"namespace", ev.Namespace,
		"name", ev.PodName,
		"reason", ev.Reason,
		"action", ev.Action,
	)
	a.enqueue(deliverJob{kind: jobEvent, ev: &ev})
}

// enqueue fans a job out to every provider queue, falling back to synchronous
// delivery before Start when no worker exists to pick it up.
func (a *AlertManager) enqueue(job deliverJob) {
	a.mu.Lock()
	started, stopped := a.started, a.stopped
	if stopped {
		a.mu.Unlock()
		return
	}
	if started {
		a.fanOut(job)
		a.mu.Unlock()
		return
	}
	entries := make([]*providerEntry, len(a.entries))
	for i := range a.entries {
		entries[i] = &a.entries[i]
	}
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	for _, entry := range entries {
		if err := a.dispatch(
			ctx, entry, job, deliverOpts{retry: entry.retry},
		); err != nil {
			klog.ErrorS(err, "failed to send",
				"provider", entry.provider.Name(), "key", job.key())
			if entry.fallback == nil {
				continue
			}
			if fbErr := a.deliverFallback(
				ctx, entry.fallback, entry.provider.Name(), job,
			); fbErr != nil {
				klog.ErrorS(fbErr, "fallback provider failed",
					"provider", entry.fallback.provider.Name())
			}
		}
	}
}

// ThreadProvider is an optional interface for providers that support
// incident-aware messaging (e.g., Slack threads).

type ThreadProvider interface {
	SendIncident(inc *model.Incident, action model.IncidentAction) error
}

// InsightThreadProvider is a ThreadProvider that can also show the insight
// engine's diagnosis — likely cause, impact, recent changes — in its own
// format. Without it a rich provider builds its message from the incident
// alone and the diagnosis is silently dropped.
type InsightThreadProvider interface {
	ThreadProvider
	SendIncidentWithInsight(
		inc *model.Incident,
		action model.IncidentAction,
		ins *insight.Insight,
	) error
}

// EventDeliveryProvider is a marker interface for providers whose real
// delivery is implemented in SendEvent (not SendMessage). PagerDuty,
// Opsgenie, Zenduty, and Email all stub SendMessage to return nil — the
// routing layer must call SendEvent instead for these providers.

type EventDeliveryProvider interface {
	Provider
	UsesEventDelivery()
}

// incidentToEvent maps a delivered incident to the legacy event.Event shape
// these EventDeliveryProvider providers' SendEvent expects.

func (a *AlertManager) NotifyIncident(
	inc *model.Incident,
	action model.IncidentAction,
	insight *insight.Insight,
) {
	if action == model.ActionSkip {
		return
	}

	if a.isSilenced(inc) {
		klog.V(4).InfoS("incident suppressed by silence rule",
			"key", inc.Key, "id", inc.ID, "reason", inc.Reason,
			"namespace", inc.Namespace)
		return
	}

	klog.InfoS(
		"sending incident",
		"action",
		action,
		"key",
		inc.Key,
		"id",
		inc.ID,
		"count",
		inc.Count,
	)

	a.mu.Lock()
	started := a.started
	stopped := a.stopped
	a.mu.Unlock()
	if stopped {
		return
	}
	if !started {
		a.deliverAllSync(inc, action, insight)
		return
	}

	snap := inc.Clone()
	ins := insight
	if ins != nil {
		cp := *ins
		ins = &cp
	}
	job := incidentJob(snap, action, ins)

	a.mu.Lock()
	stopped = a.stopped
	if !stopped {
		a.fanOut(job)
	}
	a.mu.Unlock()
}
