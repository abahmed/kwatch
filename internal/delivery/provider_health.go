package delivery

import "github.com/abahmed/kwatch/internal/metrics"

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
type ProviderHealth struct {
	Name   string
	Reason string
}

// setProviderHealth records a provider's last outcome and wakes the
// health publisher when it changed.
func (m *Manager) setProviderHealth(entry *providerEntry, reason string) {
	name := entry.lookupName()
	m.healthMu.Lock()
	if m.providerReasons == nil {
		m.providerReasons = make(map[string]string)
	}
	changed := m.providerReasons[name] != reason
	m.providerReasons[name] = reason
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
			Name: name, Reason: m.providerReasons[name],
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
	for _, name := range names {
		if reason, ok := m.providerReasons[name]; ok {
			kept[name] = reason
		}
	}
	m.providerReasons = kept
	m.healthMu.Unlock()
	m.signalProviderHealth()
}
