package delivery

import (
	"context"
	"errors"
)

// deliverFallback re-sends a job through a provider's configured fallback.
// The job shape lives in the job, so one call covers every shape; only the
// retry budget and the "primary failed" prefix differ from a normal
// delivery.
//
// The fallback has its own pacing and outage state, and its own failures
// back it off like any provider's. It is never sent to before a block it
// asked for ends. With waitBlock false that block is not waited out: the
// call waits at most one pacing interval and otherwise returns
// errProviderBlocked, so the primary's worker is not held for minutes by
// a rate-limited or backed-off fallback.
func (m *Manager) deliverFallback(
	ctx context.Context,
	entry *providerEntry,
	primary string,
	job deliverJob,
	waitBlock bool,
) error {
	if !m.wants(*entry, job) || !acceptsPagingOnly(*entry, job) ||
		(job.informational() && skipsPlainMessages(entry.provider)) {
		return errFallbackNotRouted
	}
	opts := deliverOpts{retry: fallbackRetryConfig(entry.retry)}
	if entry.provider.Name() != primary {
		opts.fallbackFrom = primary
	}
	if err := m.fallbackSlot(ctx, entry, waitBlock); err != nil {
		return err
	}
	err := m.dispatch(ctx, entry, job, opts)
	switch {
	case err == nil:
		m.recordProviderSuccess(entry)
	case ctx.Err() == nil:
		m.recordProviderFailure(entry, err)
	}
	return err
}

// fallbackSlot reserves the fallback's send slot: waiting out its block
// when waitBlock is set, or failing with errProviderBlocked otherwise.
func (m *Manager) fallbackSlot(
	ctx context.Context, entry *providerEntry, waitBlock bool,
) error {
	name := entry.provider.Name()
	if !waitBlock {
		return m.waitForShortSendSlot(ctx, name)
	}
	if !m.waitForSendSlot(ctx, name) {
		return ctx.Err()
	}
	return nil
}

// errFallbackNotRouted means the fallback's routes, or the paging rules of
// the job (PagingOnly, SkipPaging), exclude it, or the job only informs
// and the fallback skips such messages, so the job was not delivered
// anywhere.
var errFallbackNotRouted = errors.New("fallback_not_routed")
