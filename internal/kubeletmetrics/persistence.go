package kubeletmetrics

import (
	"context"
	"encoding/json"
	"time"

	"k8s.io/klog/v2"
)

func (m *Monitor) loadState(ctx context.Context) {
	if !m.cfg.PersistState || m.store == nil {
		return
	}
	data, err := m.store.LoadTelemetryState(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to load kubelet telemetry state")
		return
	}
	if len(data) == 0 {
		return
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		klog.ErrorS(err, "failed to decode kubelet telemetry state")
		return
	}
	m.mu.Lock()
	if state.Previous != nil {
		m.previous = state.Previous
	}
	if state.Failures != nil {
		m.failures = state.Failures
	}
	if state.Successes != nil {
		m.successes = state.Successes
	}
	if state.StateSeen != nil {
		m.stateSeen = state.StateSeen
	}
	if state.Baselines != nil {
		m.baselines = state.Baselines
	}
	m.mu.Unlock()
}

func (m *Monitor) saveState(ctx context.Context) {
	if !m.cfg.PersistState || m.store == nil {
		return
	}
	m.mu.Lock()
	state := persistedState{
		Previous:  cloneMetricSnapshots(m.previous),
		Failures:  cloneIntMap(m.failures),
		Successes: cloneIntMap(m.successes),
		StateSeen: cloneTimes(m.stateSeen),
		Baselines: cloneUsageBaselines(m.baselines),
	}
	m.mu.Unlock()
	data, err := json.Marshal(state)
	if err != nil {
		klog.ErrorS(err, "failed to encode kubelet telemetry state")
		return
	}
	if err := m.store.SaveTelemetryState(ctx, data); err != nil {
		klog.ErrorS(err, "failed to persist kubelet telemetry state")
	}
}

func cloneMetricSnapshots(
	source map[string]metricSnapshot,
) map[string]metricSnapshot {
	clone := make(map[string]metricSnapshot, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneUsageBaselines(
	source map[string]usageBaseline,
) map[string]usageBaseline {
	clone := make(map[string]usageBaseline, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneIntMap(source map[string]int) map[string]int {
	clone := make(map[string]int, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneTimes(source map[string]time.Time) map[string]time.Time {
	clone := make(map[string]time.Time, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
