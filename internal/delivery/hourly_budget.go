package delivery

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// After a long outage, or a cluster-wide failure, every incident would be
// announced one by one and bury the channel. Each provider therefore has
// an hourly budget of new conversations. Announcements over the budget
// are folded into the overflow summary, and so are the later updates and
// the resolve of a folded conversation, so the channel never receives a
// bare update for an incident it was not told about.
//
// Page-tier messages (route severity "critical") and the resolves of
// paged incidents are exempt. A pager skips the overflow summary, so a
// folded page would reach nobody.
//
// Updates and resolves of announced conversations are not counted: the
// queue already keeps only the newest revision of each, and a channel
// must always learn that something it was told about changed or ended.

// The per-provider budget comes from alert.<provider>.hourlyBudget and
// defaults to config.DefaultHourlyBudget; 0 disables the limit.

// hourlyBudget is a token bucket that refills budget tokens per hour.
type hourlyBudget struct {
	tokens  float64
	updated time.Time
}

// admit takes one token from the provider's budget. limit <= 0 means the
// provider has no budget.
func (p *sendPacer) admit(provider string, limit int, now time.Time) bool {
	if limit <= 0 {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hourly == nil {
		p.hourly = make(map[string]*hourlyBudget)
	}
	bucket := p.hourly[provider]
	if bucket == nil {
		bucket = &hourlyBudget{tokens: float64(limit), updated: now}
		p.hourly[provider] = bucket
	}
	if elapsed := now.Sub(bucket.updated); elapsed > 0 {
		refill := elapsed.Hours() * float64(limit)
		bucket.tokens = min(float64(limit), bucket.tokens+refill)
		bucket.updated = now
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

// foldOverBudget folds an incident job into the provider's overflow
// summary when its conversation is over budget, and reports whether it
// did. A folded job is settled: its outbox record is removed.
func (m *Manager) foldOverBudget(entry *providerEntry, job deliverJob) bool {
	if job.kind != jobIncident || job.incident == nil ||
		entry.hourlyBudget <= 0 {
		return false
	}
	name, key := entry.lookupName(), job.key()
	fate := m.openFateOf(name, key)
	if job.incident.IsPage() {
		// A page is never folded: the overflow summary is plain text,
		// which a pager skips, so the page would be lost. A conversation
		// folded at a lower tier is announced now instead.
		if fate == openFolded {
			m.setOpenFate(name, key, openLost)
		}
		return false
	}
	announces := job.opens() || fate == openLost
	if fate != openFolded {
		if !announces || m.pacer.admit(name, entry.hourlyBudget,
			m.nowTime()) {
			return false
		}
	}
	if job.isResolve() {
		m.forgetOpen(name, key)
	} else {
		m.setOpenFate(name, key, openFolded)
	}
	metrics.DefaultRegistry().Delivery.BudgetFolded.Add(1)
	klog.V(2).InfoS("notification folded into the digest",
		"component", "delivery", "operation", "hourly_budget",
		"provider", entry.provider.Name(), "key", key)
	m.addToOverflowSummary(entry.provider.Name(), job)
	m.outbox.Load().remove(job.outboxID)
	return true
}
