package probe

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (m *Monitor) record(
	key, owner, reason string, ok bool, detail string,
) {
	m.mu.Lock()
	if ok {
		if !m.failing[key] {
			// This target was never reported failing, so there is nothing to
			// recover from. Resolving anyway cost a locked engine scan per
			// healthy target per tick, on a target that had never had an
			// incident in the first place.
			delete(m.failures, key)
			delete(m.successes, key)
			m.mu.Unlock()
			return
		}
		m.failures[key] = 0
		m.successes[key]++
		if m.successes[key] < threshold(m.cfg.RecoveryThreshold, 1) {
			m.mu.Unlock()
			return
		}
		delete(m.failures, key)
		delete(m.successes, key)
		delete(m.failing, key)
	} else {
		m.successes[key] = 0
		m.failures[key]++
		if m.failures[key] < threshold(m.cfg.FailureThreshold, 1) {
			m.mu.Unlock()
			return
		}
		m.failing[key] = true
	}
	m.mu.Unlock()

	if ok {
		m.incidentSink.Resolve(probeRef(owner), reason)
		if reason == constant.ReasonActiveProbeFailure {
			m.incidentSink.Resolve(
				probeRef(owner), constant.ReasonActiveProbeLatency,
			)
		}
		return
	}
	m.incidentSink.Process(
		observe.Synthetic("activeprobe", owner, reason).
			WithSeverity(model.SeverityWarning).
			WithHint(fmt.Sprintf("probe %s failed: %s", owner, detail)),
	)
}

// probeRef is the subject an active probe's incidents are about. A probe
// target is not a Kubernetes object, so its owner name is its whole identity.
func probeRef(owner string) model.ObjectRef {
	return model.ObjectRef{Kind: "activeprobe", Name: owner}
}

func threshold(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
