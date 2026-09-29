package delivery

import (
	"context"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
)

// jobKind names what a delivery carries. Everything kwatch sends is one of
// three shapes, and every provider accepts them through the same four-way
// interface check.
type jobKind int

const (
	jobMessage jobKind = iota
	jobEvent
	jobStory
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
	return a.dispatchStory(ctx, entry, job, opts)
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
