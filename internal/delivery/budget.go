package delivery

import (
	"context"
	"sync"
	"time"
)

// A burst of incidents must not become a burst of requests: forty requests
// in one minute earn a rate-limit response from a chat provider and delay
// everything queued behind them.
//
// Two things prevent that together. Pacing spreads deliveries at a rate
// providers tolerate, with the existing queue absorbing the burst. An
// overflow summary covers the remainder: when the queue is full, what
// cannot be accepted is summarized in one message rather than dropped into
// the dead-letter queue where nobody watching the channel would see it.
const (
	// defaultSendInterval is the minimum gap between two deliveries to the
	// same provider — roughly thirty notifications a minute, comfortably
	// under every provider's published limit.
	defaultSendInterval = 2 * time.Second
	// maxOverflowSummaryReasons bounds how many distinct reasons an
	// overflow summary names.
	maxOverflowSummaryReasons = 8
)

// sendPacer holds the per-provider send cadence and overflow summaries. It is
// keyed by provider name so providerEntry stays copyable.
type sendPacer struct {
	mu        sync.Mutex
	last      map[string]time.Time
	summaries map[string]*overflowSummary
	interval  time.Duration
	// blocked is when each blocked provider may be sent to again.
	blocked map[string]time.Time
	// failures counts each provider's failed rounds in a row.
	failures map[string]int
	// hourly is each provider's notification budget.
	hourly map[string]*hourlyBudget
}

func (p *sendPacer) sendInterval() time.Duration {
	if p.interval > 0 {
		return p.interval
	}
	return defaultSendInterval
}

// reserve returns how long the caller must wait before delivering to provider,
// and records the resulting send time. Called with the pacer lock held.
func (p *sendPacer) reserve(provider string, now time.Time) time.Duration {
	if p.last == nil {
		p.last = make(map[string]time.Time)
	}
	earliest := p.last[provider].Add(p.sendInterval())
	if blocked := p.blocked[provider]; blocked.After(earliest) {
		earliest = blocked
	}
	if !earliest.After(now) {
		p.last[provider] = now
		return 0
	}
	p.last[provider] = earliest
	return earliest.Sub(now)
}

// waitForSendSlot paces deliveries to one provider. It returns false when the
// context ended while waiting, in which case the caller must not deliver.
func (m *Manager) waitForSendSlot(
	ctx context.Context,
	provider string,
) bool {
	m.pacer.mu.Lock()
	wait := m.pacer.reserve(provider, m.nowTime())
	m.pacer.mu.Unlock()
	if wait <= 0 {
		return true
	}
	return m.sleep(ctx, wait)
}

// retain keeps the pacing, outage, hourly budget and pending summary state
// of the providers in keep and drops the rest. A reconfiguration must not
// refill an exhausted budget, forget a provider outage, or lose an overflow
// summary whose notifications already left the outbox.
func (p *sendPacer) retain(keep map[string]struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	retainKeys(p.last, keep)
	retainKeys(p.summaries, keep)
	retainKeys(p.blocked, keep)
	retainKeys(p.failures, keep)
	retainKeys(p.hourly, keep)
}

func retainKeys[V any](values map[string]V, keep map[string]struct{}) {
	for name := range values {
		if _, ok := keep[name]; !ok {
			delete(values, name)
		}
	}
}
