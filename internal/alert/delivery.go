package alert

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

type deliverJob struct {
	inc     *model.Incident
	action  model.IncidentAction
	insight *insight.Insight
}

type DeadLetterEntry struct {
	Provider  string               `json:"provider"`
	Key       string               `json:"key"`
	Action    model.IncidentAction `json:"action"`
	Error     string               `json:"error"`
	Timestamp time.Time            `json:"timestamp"`
}

// deliverFallbackIncident sends an incident through the fallback's native
// delivery interface. Event-based providers deliberately implement
// SendMessage as a no-op, so a fallback must not always use that method.
func (a *AlertManager) deliverFallbackIncident(
	ctx context.Context,
	entry *providerEntry,
	primary string,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	if primary == entry.provider.Name() {
		return a.deliverFallbackIncidentWithContext(ctx, entry, primary, inc, action, ins)
	}
	var err error
	util.WithProviderContext(entry.provider.Name(), ctx, func() {
		err = a.deliverFallbackIncidentWithContext(ctx, entry, primary, inc, action, ins)
	})
	return err
}

func (a *AlertManager) deliverFallbackIncidentWithContext(
	ctx context.Context,
	entry *providerEntry,
	primary string,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	p := entry.provider
	retry := fallbackRetryConfig(entry.retry)
	if ip, ok := p.(InsightThreadProvider); ok {
		return sendWithRetry(ctx, func() error {
			return ip.SendIncidentWithInsight(inc, action, ins)
		}, retry, p.Name())
	}
	if tp, ok := p.(ThreadProvider); ok {
		return sendWithRetry(ctx, func() error {
			return tp.SendIncident(inc, action)
		}, retry, p.Name())
	}
	if _, ok := p.(EventDeliveryProvider); ok {
		ev := incidentToEvent(inc, action)
		return sendWithRetry(ctx, func() error {
			return p.SendEvent(ev)
		}, retry, p.Name())
	}
	msg := truncateMsg(
		"[fallback — primary "+primary+" failed] "+a.buildMessage(inc, action, ins, nil),
		entry.maxBytes,
	)
	return sendWithRetry(ctx, func() error {
		return p.SendMessage(msg)
	}, retry, p.Name())
}

func deliverFallbackMessage(
	ctx context.Context,
	entry *providerEntry,
	primary, msg string,
) error {
	var err error
	util.WithProviderContext(entry.provider.Name(), ctx, func() {
		err = deliverFallbackMessageWithContext(ctx, entry, primary, msg)
	})
	return err
}

func deliverFallbackMessageWithContext(
	ctx context.Context,
	entry *providerEntry,
	primary, msg string,
) error {
	p := entry.provider
	retry := fallbackRetryConfig(entry.retry)
	if _, ok := p.(EventDeliveryProvider); ok {
		ev := &event.Event{PodName: msg, Reason: constant.ReasonNotify}
		return sendWithRetry(ctx, func() error { return p.SendEvent(ev) }, retry, p.Name())
	}
	fallback := truncateMsg(
		"[fallback — primary "+primary+" failed] "+msg,
		entry.maxBytes,
	)
	return sendWithRetry(ctx, func() error { return p.SendMessage(fallback) }, retry, p.Name())
}

func deliverFallbackEvent(
	ctx context.Context,
	entry *providerEntry,
	ev *event.Event,
) error {
	var err error
	util.WithProviderContext(entry.provider.Name(), ctx, func() {
		err = deliverFallbackEventWithContext(ctx, entry, ev)
	})
	return err
}

func deliverFallbackEventWithContext(
	ctx context.Context,
	entry *providerEntry,
	ev *event.Event,
) error {
	p := entry.provider
	retry := fallbackRetryConfig(entry.retry)
	return sendWithRetry(ctx, func() error { return p.SendEvent(ev) }, retry, p.Name())
}

const channelCap = 256
const dlqCap = 100

const defaultMaxBackoff = 30 * time.Second

func fallbackRetryConfig(rc retryConfig) retryConfig {
	if rc.maxAttempts < 1 {
		rc.maxAttempts = 1
	}
	return rc
}

func (a *AlertManager) recordDeadLetter(
	entry *providerEntry,
	inc *model.Incident,
	action model.IncidentAction,
	err error,
) {
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	a.dlqRing[a.dlqHead] = DeadLetterEntry{
		Provider:  entry.provider.Name(),
		Key:       string(inc.Key),
		Action:    action,
		Error:     err.Error(),
		Timestamp: a.nowTime(),
	}
	a.dlqHead = (a.dlqHead + 1) % dlqCap
}

// deliverOne handles the full send+retry for a single (entry, incident) pair.

func (a *AlertManager) deliverOne(
	ctx context.Context,
	entry *providerEntry,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) {
	util.WithProviderContext(entry.provider.Name(), ctx, func() {
		a.deliverOneWithContext(ctx, entry, inc, action, ins)
	})
}

func (a *AlertManager) deliverOneWithContext(
	ctx context.Context,
	entry *providerEntry,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) {
	p := entry.provider
	metrics.Default.NotificationsTotal.Add(1)

	tpl := entry.templates
	if len(tpl) == 0 {
		tpl = a.globalTemplates()
	}

	// Evaluate routes before rendering so route-filtered incidents don't pay
	// for message building (routes depend only on the incident, not the
	// message).
	if !shouldDeliver(entry.routes, inc) {
		klog.V(4).InfoS("incident filtered by route",
			"provider", p.Name(),
			"key", inc.Key)
		return
	}

	raw := a.buildMessage(inc, action, ins, tpl)
	msg := truncateMsg(raw, entry.maxBytes)

	var err error
	if ip, ok := p.(InsightThreadProvider); ok {
		sendInc := inc
		if entry.maxBytes > 0 {
			sendInc = a.clampIncidentForProvider(
				inc,
				action,
				ins,
				entry.maxBytes,
				tpl,
				len(raw),
			)
		}
		err = sendWithRetry(ctx, func() error {
			return ip.SendIncidentWithInsight(sendInc, action, ins)
		}, entry.retry, p.Name())
	} else if tp, ok := p.(ThreadProvider); ok {
		sendInc := inc
		if entry.maxBytes > 0 {
			sendInc = a.clampIncidentForProvider(
				inc,
				action,
				ins,
				entry.maxBytes,
				tpl,
				len(raw),
			)
		}
		err = sendWithRetry(ctx, func() error {
			return tp.SendIncident(sendInc, action)
		}, entry.retry, p.Name())
	} else if _, ok := p.(EventDeliveryProvider); ok {
		ev := incidentToEvent(inc, action)
		err = sendWithRetry(ctx, func() error {
			return p.SendEvent(ev)
		}, entry.retry, p.Name())
	} else {
		err = sendWithRetry(ctx, func() error {
			return p.SendMessage(msg)
		}, entry.retry, p.Name())
	}
	if err != nil {
		metrics.Default.NotificationsDropped.Add(1)
		klog.ErrorS(
			err,
			"failed to send",
			"provider",
			p.Name(),
			"key",
			inc.Key,
			"id",
			inc.ID,
		)
		a.recordDeadLetter(entry, inc, action, err)
		if entry.fallback != nil {
			fbErr := a.deliverFallbackIncident(
				ctx, entry.fallback, p.Name(), inc, action, ins,
			)
			if fbErr != nil {
				klog.ErrorS(
					fbErr,
					"fallback delivery failed",
					"provider",
					entry.fallback.provider.Name(),
				)
			}
		}
	}
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
			metrics.Default.NotificationsDropped.Add(1)
			a.recordDeadLetter(
				&entry,
				job.inc,
				job.action,
				fmt.Errorf("delivery queue saturated"),
			)
		}
	}
}

// deliverAllSync sends directly to every provider (synchronous).
// Used before Start() is called (e.g. kwatch replay).

func (a *AlertManager) deliverAllSync(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) {
	for _, entry := range a.entries {
		p := entry.provider
		tpl := entry.templates
		if len(tpl) == 0 {
			tpl = a.globalTemplates()
		}
		if !shouldDeliver(entry.routes, inc) {
			continue
		}
		raw := a.buildMessage(inc, action, ins, tpl)
		msg := truncateMsg(raw, entry.maxBytes)
		var err error
		if ip, ok := p.(InsightThreadProvider); ok {
			sendInc := inc
			if entry.maxBytes > 0 {
				sendInc = a.clampIncidentForProvider(
					inc,
					action,
					ins,
					entry.maxBytes,
					tpl,
					len(raw),
				)
			}
			err = sendWithRetry(context.Background(), func() error {
				return ip.SendIncidentWithInsight(sendInc, action, ins)
			}, entry.retry, p.Name())
		} else if tp, ok := p.(ThreadProvider); ok {
			sendInc := inc
			if entry.maxBytes > 0 {
				sendInc = a.clampIncidentForProvider(
					inc,
					action,
					ins,
					entry.maxBytes,
					tpl,
					len(raw),
				)
			}
			err = sendWithRetry(context.Background(), func() error {
				return tp.SendIncident(sendInc, action)
			}, entry.retry, p.Name())
		} else if _, ok := p.(EventDeliveryProvider); ok {
			ev := incidentToEvent(inc, action)
			err = sendWithRetry(context.Background(), func() error {
				return p.SendEvent(ev)
			}, entry.retry, p.Name())
		} else {
			err = sendWithRetry(context.Background(), func() error {
				return p.SendMessage(msg)
			}, entry.retry, p.Name())
		}
		if err != nil {
			metrics.Default.NotificationsDropped.Add(1)
			klog.ErrorS(
				err,
				"sync delivery failed",
				"provider",
				p.Name(),
				"key",
				inc.Key,
				"id",
				inc.ID,
			)
			if entry.fallback != nil {
				if fbErr := a.deliverFallbackIncident(
					context.Background(), entry.fallback, p.Name(), inc, action, ins,
				); fbErr != nil {
					klog.ErrorS(fbErr, "sync fallback delivery failed",
						"provider", entry.fallback.provider.Name())
				}
			}
		}
	}
}
