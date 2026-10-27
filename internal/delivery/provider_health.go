package delivery

import (
	"time"

	"github.com/abahmed/kwatch/internal/metrics"
)

// Provider health reasons. They are a fixed vocabulary so /health never
// carries a provider's error text.
const (
	reasonUnavailable = "provider_unavailable"
	reasonRejected    = "provider_rejected"
	reasonRateLimited = "provider_rate_limited"
)

// ProviderHealth is the last delivery outcome of one provider. Reason is
// empty when its last delivery succeeded (or none was tried yet), else
// one of provider_unavailable, provider_rejected, provider_rate_limited.
// LastSuccess and LastError are kept independently; LastErrorMessage is
// a short, redacted reason for the last failure (see errorMessage).
type ProviderHealth struct {
	Name             string
	Reason           string
	LastSuccess      time.Time
	LastError        time.Time
	LastErrorMessage string
}

// providerOutcome is the delivery history ProviderHealth reports.
type providerOutcome struct {
	lastSuccess  time.Time
	lastError    time.Time
	errorMessage string
}

// setProviderHealth records a provider's last outcome and wakes the
// health publisher when it changed. err is nil on success.
func (m *Manager) setProviderHealth(
	entry *providerEntry, reason string, err error,
) {
	name := entry.lookupName()
	now := m.now()
	m.healthMu.Lock()
	if m.providerReasons == nil {
		m.providerReasons = make(map[string]string)
	}
	if m.providerOutcomes == nil {
		m.providerOutcomes = make(map[string]providerOutcome)
	}
	changed := m.providerReasons[name] != reason
	m.providerReasons[name] = reason
	outcome := m.providerOutcomes[name]
	if reason == "" {
		outcome.lastSuccess = now
	} else {
		outcome.lastError = now
		outcome.errorMessage = errorMessage(reason, err)
	}
	m.providerOutcomes[name] = outcome
	m.healthMu.Unlock()
	if changed {
		m.signalProviderHealth()
	}
}

func (m *Manager) signalProviderHealth() {
	select {
	case m.healthEventsChannel() <- struct{}{}:
	default:
	}
}

func (m *Manager) healthEventsChannel() chan struct{} {
	m.healthMu.Lock()
	defer m.healthMu.Unlock()
	if m.healthEvents == nil {
		m.healthEvents = make(chan struct{}, 1)
	}
	return m.healthEvents
}

// ProviderHealthEvents is signalled when a provider's health changed or
// the configured providers did. Read ProviderHealth after a signal.
func (m *Manager) ProviderHealthEvents() <-chan struct{} {
	return m.healthEventsChannel()
}

// ProviderHealth returns the configured providers' last outcomes in
// configuration order. A provider outage never fails readiness: kwatch
// keeps watching and queues for the provider.
func (m *Manager) ProviderHealth() []ProviderHealth {
	m.mu.Lock()
	var names []string
	if generation := m.currentGenerationLocked(); generation != nil {
		names = append(names, generation.order...)
	}
	m.mu.Unlock()
	m.healthMu.Lock()
	defer m.healthMu.Unlock()
	out := make([]ProviderHealth, 0, len(names))
	for _, name := range names {
		out = append(out, ProviderHealth{
			Name:             name,
			Reason:           m.providerReasons[name],
			LastSuccess:      m.providerOutcomes[name].lastSuccess,
			LastError:        m.providerOutcomes[name].lastError,
			LastErrorMessage: m.providerOutcomes[name].errorMessage,
		})
	}
	return out
}

// publishQueueDepthLocked reports a provider's waiting jobs: its queue
// and its restored backlog. The caller holds m.mu.
func (m *Manager) publishQueueDepthLocked(entry providerEntry) {
	depth := len(m.backlog[entry.lookupName()])
	if entry.ch != nil {
		depth += len(entry.ch)
	}
	metrics.DefaultRegistry().Delivery.SetQueueDepth(
		entry.lookupName(), depth)
}

// publishQueueDepth is publishQueueDepthLocked for callers without m.mu.
func (m *Manager) publishQueueDepth(entry providerEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishQueueDepthLocked(entry)
}

// configureProviderMetrics bounds the queue depth labels to the
// configured providers and forgets the health of removed ones.
func (m *Manager) configureProviderMetrics(names []string) {
	metrics.DefaultRegistry().Delivery.ConfigureQueueProviders(names)
	m.healthMu.Lock()
	kept := make(map[string]string, len(names))
	keptOutcomes := make(map[string]providerOutcome, len(names))
	for _, name := range names {
		if reason, ok := m.providerReasons[name]; ok {
			kept[name] = reason
		}
		if o, ok := m.providerOutcomes[name]; ok {
			keptOutcomes[name] = o
		}
	}
	m.providerReasons = kept
	m.providerOutcomes = keptOutcomes
	m.healthMu.Unlock()
	m.signalProviderHealth()
}
