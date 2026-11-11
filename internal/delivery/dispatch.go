package delivery

import (
	"context"
	"errors"
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
	// sent, when set, receives the text of an incident message as it
	// was prepared for the provider, for the send trail.
	sent *string
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
	attempts, sent := 0, ""
	opts.attempts, opts.sent = &attempts, &sent
	var err error
	if job.kind == jobIncident && job.incident != nil {
		err = m.dispatchIncident(ctx, entry, job, opts)
	} else {
		err = m.dispatchMessage(ctx, entry, job, opts)
	}
	record := newSendRecord(entry.provider.Name(), job, opts, attempts, err)
	record.pager, record.text = isPager(entry), sent
	logSend(record)
	m.noteSent(record, job)
	if err != nil && !endedByShutdown(ctx, err) {
		m.failures.add(m.nowTime())
	}
	return err
}

// endedByShutdown reports whether a failed send was cut off by shutdown
// (the worker's context or the send context was cancelled) and not by the
// provider: it is not a failure of the provider.
func endedByShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled)
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
	return sendWithRetry(ctx, opts.counted(func() error {
		requestCtx, done := m.beginSend(ctx)
		defer done()
		return p.SendMessage(requestCtx, msg)
	}), opts.retry, p.Name())
}
