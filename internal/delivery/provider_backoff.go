package delivery

import (
	"context"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// A provider that is down, slow or rate limiting is waited out instead of
// losing what was queued for it. The wait belongs to the provider, not to
// one job: while a provider is blocked every job queued for it waits, in
// order, and a job's own retry attempts are not spent on the wait.
const (
	// initialProviderBackoff is the first wait after an outage.
	initialProviderBackoff = 5 * time.Second
	// maxProviderBackoff caps the doubling wait during a long outage and
	// any wait a provider asks for.
	maxProviderBackoff = 5 * time.Minute
)

// rateLimitJitter spreads the wait after a rate limit that named none by
// up to this fraction either way.
const rateLimitJitter = 0.2

// providerBackoff is the wait after the given number of failed rounds in
// a row: 5s, 10s, 20s ... capped at maxProviderBackoff.
func providerBackoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	return backoffFor(failures, initialProviderBackoff, maxProviderBackoff)
}

// block keeps the provider from being sent to until until.
func (p *sendPacer) block(provider string, until time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.blocked == nil {
		p.blocked = make(map[string]time.Time)
	}
	if until.After(p.blocked[provider]) {
		p.blocked[provider] = until
	}
}

// recordOutage counts one more failed round and returns the wait before
// the next one.
func (p *sendPacer) recordOutage(provider string) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failures == nil {
		p.failures = make(map[string]int)
	}
	p.failures[provider]++
	return providerBackoff(p.failures[provider])
}

// recordRecovery resets the outage backoff once the provider accepts a
// job again.
func (p *sendPacer) recordRecovery(provider string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.failures, provider)
	delete(p.rejections, provider)
}

// rejectionsBeforeNotice is how many rejections in a row, none of them
// about the credential or address, show that the provider refuses
// everything and not one malformed payload.
const rejectionsBeforeNotice = 3

// rejectionMarksProvider counts a permanent failure and reports whether
// it says the provider's webhook, token or address is wrong: an answer
// of 401, 403, 404 or 410, a refused credential, or several rejections in
// a row. One 400 for one payload says nothing about the provider.
func (p *sendPacer) rejectionMarksProvider(
	provider string, err error,
) bool {
	if refusedAccess(err) {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rejections == nil {
		p.rejections = make(map[string]int)
	}
	p.rejections[provider]++
	return p.rejections[provider] >= rejectionsBeforeNotice
}

// refusedAccess reports whether err says the credential or the address
// is bad, not the payload.
func refusedAccess(err error) bool {
	if code, ok := transport.StatusOf(err); ok {
		switch code {
		case http.StatusUnauthorized, http.StatusForbidden,
			http.StatusNotFound, http.StatusGone:
			return true
		}
		return false
	}
	return hasAny(strings.ToLower(err.Error()), tokenHints)
}

// blockedUntil returns when the provider may be sent to again.
func (p *sendPacer) blockedUntil(provider string) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blocked[provider]
}

// recordProviderFailure blocks the provider after a failed delivery: for
// the wait it asked for when it rate limited, for the outage backoff
// when it failed in a way that passes. A rejection does not block it, and
// marks the provider rejecting only when it is not about one payload (see
// rejectionMarksProvider).
func (m *Manager) recordProviderFailure(entry *providerEntry, err error) {
	name := entry.provider.Name()
	now := m.nowTime()
	if wait, limited := serverRetryAfter(err); limited {
		if wait <= 0 {
			// No wait was named: back off like an outage, with jitter
			// so several pods do not retry together.
			wait = m.pacer.recordOutage(name)
			wait = min(applyJitter(wait, rateLimitJitter),
				maxProviderBackoff)
		}
		m.pacer.block(name, now.Add(wait))
		m.setProviderHealth(entry, reasonRateLimited, err)
		klog.InfoS("provider rate limited delivery; waiting",
			"component", "delivery", "operation", "rate_limit",
			"provider", name, "wait", wait)
		return
	}
	if transport.IsPermanent(err) {
		if m.pacer.rejectionMarksProvider(name, err) {
			m.setProviderHealth(entry, reasonRejected, err)
		}
		return
	}
	// Jittered like the rate-limit backoff, so the pods of several
	// clusters sharing one provider do not all come back at once.
	wait := min(applyJitter(m.pacer.recordOutage(name), rateLimitJitter),
		maxProviderBackoff)
	m.pacer.block(name, now.Add(wait))
	m.setProviderHealth(entry, reasonUnavailable, err)
	klog.InfoS("provider unavailable; keeping its queue",
		"component", "delivery", "operation", "backoff",
		"provider", name, "wait", wait)
}

// recordProviderSuccess ends an outage.
func (m *Manager) recordProviderSuccess(entry *providerEntry) {
	m.pacer.recordRecovery(entry.provider.Name())
	m.setProviderHealth(entry, "", nil)
}

// sleep waits for d unless ctx ends first. Tests replace it to move a
// fake clock instead of waiting.
func (m *Manager) sleep(ctx context.Context, d time.Duration) bool {
	if m.sleepFn != nil {
		return m.sleepFn(ctx, d)
	}
	return sleepWithContext(ctx, d) == nil
}
