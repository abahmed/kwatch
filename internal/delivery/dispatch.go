package delivery

import (
	"context"
	"text/template"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

// jobKind names what a delivery carries. Everything kwatch sends is one of
// three shapes, and every provider accepts them through the same four-way
// interface check.
type jobKind int

const (
	jobIncident jobKind = iota
	jobMessage
	jobEvent
)

// deliverOpts is what differs between the delivery paths: the retry budget,
// whether a fallback provider may be tried, and the text prefix a fallback
// adds. The dispatch itself does not vary, and used to be written out three
// times -- once for normal delivery, once for the synchronous pre-Start path,
// once for fallbacks. Three copies of a four-way type switch is three places
// for a provider kind to be handled slightly differently.
type deliverOpts struct {
	retry  retryConfig
	prefix string
}

// dispatch sends one job to one provider, choosing the richest interface the
// provider implements. It is the single place that decides how a provider is
// called.
func (a *Manager) dispatch(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	opts deliverOpts,
) error {
	p := entry.provider
	switch job.kind {
	case jobMessage:
		return a.dispatchMessage(ctx, entry, job, opts)
	case jobEvent:
		return sendWithRetry(ctx, func() error {
			return sendEvent(ctx, p, job.ev)
		}, opts.retry, p.Name())
	}
	return a.dispatchIncident(ctx, entry, job, opts)
}

func (a *Manager) dispatchMessage(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	opts deliverOpts,
) error {
	p := entry.provider
	msg := opts.prefix + job.msg
	if _, ok := p.(EventDeliveryProvider); ok {
		ev := &event.Event{PodName: msg, Reason: constant.ReasonNotify}
		return sendWithRetry(ctx, func() error {
			return sendEvent(ctx, p, ev)
		}, opts.retry, p.Name())
	}
	truncated := msg
	if entry.maxBytes > 0 {
		truncated = truncateMsg(msg, entry.maxBytes)
	}
	return sendWithRetry(ctx, func() error {
		return sendMessage(ctx, p, truncated)
	}, opts.retry, p.Name())
}

func (a *Manager) dispatchIncident(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	opts deliverOpts,
) error {
	p := entry.provider
	tpl := entry.templates
	if len(tpl) == 0 {
		tpl = a.globalTemplates()
	}
	if sp, ok := p.(StructuredNotificationProvider); ok {
		n := a.buildNotification(job.inc, job.action, job.insight)
		return sendWithRetry(ctx, func() error {
			return sp.SendNotification(ctx, n)
		}, opts.retry, p.Name())
	}
	if ip, ok := p.(InsightThreadProvider); ok {
		inc := a.fitIncident(entry, job, tpl)
		return sendWithRetry(ctx, func() error {
			return ip.SendIncidentWithInsight(
				ctx, inc, job.action, job.insight,
			)
		}, opts.retry, p.Name())
	}
	if tp, ok := p.(ThreadProvider); ok {
		inc := a.fitIncident(entry, job, tpl)
		return sendWithRetry(ctx, func() error {
			return tp.SendIncident(ctx, inc, job.action)
		}, opts.retry, p.Name())
	}
	if _, ok := p.(EventDeliveryProvider); ok {
		raw := a.buildMessage(job.inc, job.action, job.insight, tpl)
		narrative := opts.prefix + raw
		if entry.maxBytes > 0 {
			narrative = truncateMsg(narrative, entry.maxBytes)
		}
		ev := incidentToEvent(job.inc, job.action, narrative)
		return sendWithRetry(ctx, func() error {
			return sendEvent(ctx, p, ev)
		}, opts.retry, p.Name())
	}
	raw := a.buildMessage(job.inc, job.action, job.insight, tpl)
	msg := opts.prefix + raw
	if entry.maxBytes > 0 {
		msg = truncateMsg(msg, entry.maxBytes)
	}
	return sendWithRetry(ctx, func() error {
		return sendMessage(ctx, p, msg)
	}, opts.retry, p.Name())
}

func (a *Manager) buildNotification(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) *message.Notification {
	rb := message.NewReportBuilderWithPolicy(
		a.clusterName, clock.Func(a.nowTime),
		a.includePrivateLogAddresses,
	)
	report := rb.Build(inc, action, ins)
	pattern, confidence, evidence := "", 0.0, 0
	if ins != nil {
		pattern = ins.Pattern
		confidence = ins.Confidence
		evidence = len(ins.Evidence)
	}
	return message.NotificationFromReport(
		report, inc, pattern, confidence, evidence,
	)
}

func sendEvent(
	ctx context.Context,
	provider Provider,
	event *event.Event,
) error {
	return provider.SendEvent(ctx, event)
}

func sendMessage(
	ctx context.Context,
	provider Provider,
	message string,
) error {
	return provider.SendMessage(ctx, message)
}

// fitIncident trims evidence so a thread provider's single message fits the
// provider's byte limit.
func (a *Manager) fitIncident(
	entry *providerEntry,
	job deliverJob,
	tpl map[string]*template.Template,
) *model.Incident {
	clean := job.inc.Clone()
	clean.Events = ""
	clean.IncludeEvents = false
	if entry.maxBytes <= 0 {
		return clean
	}
	raw := a.buildMessage(clean, job.action, job.insight, tpl)
	return a.clampIncidentForProvider(
		clean, job.action, job.insight, entry.maxBytes, tpl, len(raw),
	)
}

// incidentJob builds the delivery for one incident.
func incidentJob(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) deliverJob {
	return deliverJob{kind: jobIncident, inc: inc, action: action, insight: ins}
}
