package alert

import (
	"context"
	"text/template"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
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
func (a *AlertManager) dispatch(
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
			return p.SendEvent(job.ev)
		}, opts.retry, p.Name())
	}
	return a.dispatchIncident(ctx, entry, job, opts)
}

func (a *AlertManager) dispatchMessage(
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
			return p.SendEvent(ev)
		}, opts.retry, p.Name())
	}
	truncated := truncateMsg(msg, entry.maxBytes)
	return sendWithRetry(ctx, func() error {
		return p.SendMessage(truncated)
	}, opts.retry, p.Name())
}

func (a *AlertManager) dispatchIncident(
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
	if ip, ok := p.(InsightThreadProvider); ok {
		inc := a.fitIncident(entry, job, tpl)
		return sendWithRetry(ctx, func() error {
			return ip.SendIncidentWithInsight(inc, job.action, job.insight)
		}, opts.retry, p.Name())
	}
	if tp, ok := p.(ThreadProvider); ok {
		inc := a.fitIncident(entry, job, tpl)
		return sendWithRetry(ctx, func() error {
			return tp.SendIncident(inc, job.action)
		}, opts.retry, p.Name())
	}
	if _, ok := p.(EventDeliveryProvider); ok {
		ev := incidentToEvent(job.inc, job.action)
		return sendWithRetry(ctx, func() error {
			return p.SendEvent(ev)
		}, opts.retry, p.Name())
	}
	raw := a.buildMessage(job.inc, job.action, job.insight, tpl)
	msg := truncateMsg(opts.prefix+raw, entry.maxBytes)
	return sendWithRetry(ctx, func() error {
		return p.SendMessage(msg)
	}, opts.retry, p.Name())
}

// fitIncident trims evidence so a thread provider's single message fits the
// provider's byte limit.
func (a *AlertManager) fitIncident(
	entry *providerEntry,
	job deliverJob,
	tpl map[string]*template.Template,
) *model.Incident {
	if entry.maxBytes <= 0 {
		return job.inc
	}
	raw := a.buildMessage(job.inc, job.action, job.insight, tpl)
	return a.clampIncidentForProvider(
		job.inc, job.action, job.insight, entry.maxBytes, tpl, len(raw),
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
