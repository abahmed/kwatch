package delivery

import (
	"context"
	"fmt"

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
func (a *Manager) Notify(msg string) {
	klog.InfoS("sending message", "messageLength", len(msg))
	a.enqueue(deliverJob{kind: jobMessage, msg: msg})
}

// NotifyEvent queues a legacy event for every provider.
func (a *Manager) NotifyEvent(ev event.Event) {
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

// enqueue retains jobs until workers exist. It never performs provider I/O on
// the caller's goroutine before Start.
func (a *Manager) enqueue(job deliverJob) {
	a.mu.Lock()
	if a.stopped {
		if a.reconfiguring && len(a.pending) < channelCap {
			a.pending = append(a.pending, job)
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()
		return
	}
	if a.started {
		a.fanOut(job)
		a.mu.Unlock()
		return
	}
	if len(a.pending) >= channelCap {
		generation := a.currentGenerationLocked()
		if generation != nil && len(generation.order) > 0 {
			entry := generation.entries[generation.order[0]]
			a.recordDeadLetter(
				&entry, job, fmt.Errorf("pending delivery queue saturated"),
			)
		}
		a.mu.Unlock()
		return
	}
	a.pending = append(a.pending, job)
	a.mu.Unlock()
}

// ThreadProvider is an optional interface for providers that support
// incident-aware messaging (e.g., Slack threads).

type ThreadProvider interface {
	SendIncident(
		ctx context.Context,
		inc *model.Incident,
		action model.IncidentAction,
	) error
}

// InsightThreadProvider is a ThreadProvider that can also show the insight
// engine's diagnosis — likely cause, impact, recent changes — in its own
// format. Without it a rich provider builds its message from the incident
// alone and the diagnosis is silently dropped.
type InsightThreadProvider interface {
	ThreadProvider
	SendIncidentWithInsight(
		ctx context.Context,
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

func (a *Manager) NotifyIncident(
	inc *model.Incident,
	action model.IncidentAction,
	insightValue *insight.Insight,
) {
	if inc == nil {
		klog.ErrorS(nil, "cannot deliver a nil incident")
		return
	}
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

	snap := inc.Clone()
	ins := insightValue
	if ins != nil {
		copy := *ins
		ins = &copy
	}
	job := incidentJob(snap, action, ins)
	a.mu.Lock()
	started := a.started
	stopped := a.stopped
	reconfiguring := a.reconfiguring
	a.mu.Unlock()
	if stopped && !reconfiguring {
		return
	}
	if !started || reconfiguring {
		a.enqueue(job)
		return
	}

	a.mu.Lock()
	stopped = a.stopped
	if !stopped {
		a.fanOut(job)
	}
	a.mu.Unlock()
}
