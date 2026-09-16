package rbac

import (
	"context"
	"time"
)

// Start performs an initial permission audit and then refreshes it at the
// deliberately slow interval used by the security monitor.
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	configured := m.configured
	m.mu.Unlock()
	if !configured {
		m.mu.Lock()
		m.status.State = "unavailable"
		m.status.Available = false
		m.mu.Unlock()
		return nil
	}
	if m.client == nil {
		return nil
	}
	const checkInterval = 15 * time.Minute
	m.check(ctx)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.check(ctx)
		}
	}
}
