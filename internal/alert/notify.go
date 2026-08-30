package alert

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// Notify sends string msg to all providers

func (a *AlertManager) Notify(msg string) {
	klog.InfoS("sending message", "msg", msg)

	a.mu.Lock()
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	ctx := a.ctx
	stopped := a.stopped
	a.mu.Unlock()
	if stopped {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for _, entry := range entries {
		p := entry.provider
		if _, ok := p.(EventDeliveryProvider); ok {
			ev := &event.Event{
				PodName: msg,
				Reason:  constant.ReasonNotify,
			}
			if err := sendWithRetry(ctx, func() error {
				return p.SendEvent(ev)
			}, entry.retry, p.Name()); err != nil && entry.fallback != nil {
				if fbErr := deliverFallbackMessage(ctx, entry.fallback, p.Name(), msg); fbErr != nil {
					klog.ErrorS(
						fbErr,
						"fallback provider failed",
						"provider",
						entry.fallback.provider.Name(),
					)
				}
			}
			continue
		}
		truncMsg := truncateMsg(msg, entry.maxBytes)
		if err := sendWithRetry(ctx, func() error {
			return p.SendMessage(truncMsg)
		}, entry.retry, p.Name()); err != nil && entry.fallback != nil {
			if fbErr := deliverFallbackMessage(ctx, entry.fallback, p.Name(), truncMsg); fbErr != nil {
				klog.ErrorS(
					fbErr,
					"fallback provider failed",
					"provider",
					entry.fallback.provider.Name(),
				)
			}
		}
	}
}

// NotifyEvent sends event to all providers

func (a *AlertManager) NotifyEvent(event event.Event) {
	klog.InfoS("sending event", "event", event)

	a.mu.Lock()
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	ctx := a.ctx
	stopped := a.stopped
	a.mu.Unlock()
	if stopped {
		return
	}

	if ctx == nil {
		ctx = context.Background()
	}
	for _, entry := range entries {
		p := entry.provider
		if err := sendWithRetry(ctx, func() error {
			return p.SendEvent(&event)
		}, entry.retry, p.Name()); err != nil && entry.fallback != nil {
			if ferr := deliverFallbackEvent(ctx, entry.fallback, &event); ferr != nil {
				klog.ErrorS(
					ferr,
					"fallback provider send failed",
					"primary",
					p.Name(),
					"fallback",
					entry.fallback.provider.Name(),
				)
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
	job := deliverJob{inc: snap, action: action, insight: ins}

	a.mu.Lock()
	stopped = a.stopped
	if !stopped {
		a.fanOut(job)
	}
	a.mu.Unlock()
}
