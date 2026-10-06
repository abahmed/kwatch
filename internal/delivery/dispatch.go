package delivery

import (
	"context"
)

// jobKind names what a delivery carries: a plain operator message or an
// incident message.
type jobKind int

const (
	jobMessage jobKind = iota
	jobIncident
)

// deliverOpts is what differs between the delivery paths: the retry budget
// and, for a fallback delivery, the name of the primary that failed. The
// dispatch itself does not vary between normal, pre-Start and fallback
// delivery, so a provider is always called the same way.
type deliverOpts struct {
	retry        retryConfig
	fallbackFrom string
	// attempts, when set, counts the provider calls the send made, so
	// the send log can say how many retries it took.
	attempts *int
}

// counted wraps one provider call so opts.attempts counts it.
func (o deliverOpts) counted(send func() error) func() error {
	return func() error {
		if o.attempts != nil {
			*o.attempts++
		}
		return send()
	}
}

// dispatch sends one job to one provider. It is the single place that
// decides how a provider is called.
func (m *Manager) dispatch(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	opts deliverOpts,
) error {
	attempts := 0
	opts.attempts = &attempts
	var err error
	if job.kind == jobIncident && job.incident != nil {
		err = m.dispatchIncident(ctx, entry, job, opts)
	} else {
		err = m.dispatchMessage(ctx, entry, job, opts)
	}
	logSend(newSendRecord(entry.provider.Name(), job, opts, attempts, err))
	return err
}

// dispatchMessage sends a plain operator message as text, cut to the
// provider's payload limit.
func (m *Manager) dispatchMessage(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	opts deliverOpts,
) error {
	p := entry.provider
	msg := job.msg
	if opts.fallbackFrom != "" {
		msg = "[fallback — primary " + opts.fallbackFrom + " failed] " + msg
	}
	if entry.maxBytes > 0 {
		msg = truncateMsg(msg, entry.maxBytes)
	}
	requestCtx := m.requestContext(ctx)
	return sendWithRetry(ctx, opts.counted(func() error {
		return p.SendMessage(requestCtx, msg)
	}), opts.retry, p.Name())
}
