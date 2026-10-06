package delivery

import (
	"context"
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
}

// blockedUntil returns when the provider may be sent to again.
func (p *sendPacer) blockedUntil(provider string) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blocked[provider]
}

// recordProviderFailure blocks the provider after a failed delivery: for
// the wait it asked for when it rate limited, for the outage backoff
// when it failed in a way that passes. A rejection does not block it.
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
		m.setProviderHealth(entry, reasonRateLimited)
		klog.InfoS("provider rate limited delivery; waiting",
			"component", "delivery", "operation", "rate_limit",
			"provider", name, "wait", wait)
		return
	}
	if transport.IsPermanent(err) {
		m.setProviderHealth(entry, reasonRejected)
		return
	}
	wait := m.pacer.recordOutage(name)
	m.pacer.block(name, now.Add(wait))
	m.setProviderHealth(entry, reasonUnavailable)
	klog.InfoS("provider unavailable; keeping its queue",
		"component", "delivery", "operation", "backoff",
		"provider", name, "wait", wait)
}

// recordProviderSuccess ends an outage.
func (m *Manager) recordProviderSuccess(entry *providerEntry) {
	m.pacer.recordRecovery(entry.provider.Name())
	m.setProviderHealth(entry, "")
}

// sleep waits for d unless ctx ends first. Tests replace it to move a
// fake clock instead of waiting.
func (m *Manager) sleep(ctx context.Context, d time.Duration) bool {
	if m.sleepFn != nil {
		return m.sleepFn(ctx, d)
	}
	return sleepWithContext(ctx, d) == nil
}
