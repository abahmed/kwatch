package rbac

import (
	"context"
	"time"
)

// Start performs an initial permission audit and then refreshes it at the
// deliberately slow interval used by the security monitor.
func (m *Monitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	configured := m.configured
	m.mu.Unlock()
	if !configured {
		m.mu.Lock()
		m.status.State = "unavailable"
		m.status.Available = false
		m.mu.Unlock()
		return
	}
	if m.client == nil {
		return
	}
	const checkInterval = 15 * time.Minute
	m.check(ctx)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}
