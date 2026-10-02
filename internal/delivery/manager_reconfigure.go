package delivery

import (
	"context"
)

// IsReconfiguring reports whether workers are stopping as part of a runtime
// generation replacement rather than application shutdown.
func (m *Manager) IsReconfiguring() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.reconfiguring()
}

func (m *Manager) finishReconfiguration(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.state.reconfiguring() {
		return
	}
	m.setStateLocked(m.state.reconfigureEnded())
	m.reconfigure.err = err
	if m.reconfigure.done != nil {
		close(m.reconfigure.done)
	}
	if err != nil && m.workers.count == 0 {
		m.closeManagerDoneLocked()
	} else if m.reconfigureEvents != nil {
		select {
		case m.reconfigureEvents <- struct{}{}:
		default:
		}
	}
}

// WaitForReconfiguration waits until an in-progress generation replacement
// has either started the new workers or failed.
func (m *Manager) WaitForReconfiguration(ctx context.Context) error {
	m.mu.Lock()
	done := m.reconfigure.done
	m.mu.Unlock()
	if done == nil {
		return ErrNoReconfiguration
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		return m.collectReconfiguration()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// collectReconfiguration returns the result of the finished replacement
// and forgets it, so the next wait reports ErrNoReconfiguration.
func (m *Manager) collectReconfiguration() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconfigure.done = nil
	return m.reconfigure.err
}

// ReconfigurationEvents notifies the application when a generation replacement
// has completed. It is separate from Done, which represents manager shutdown.
func (m *Manager) ReconfigurationEvents() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLifecycleChannelsLocked()
	return m.reconfigureEvents
}

func (m *Manager) ensureLifecycleChannelsLocked() {
	m.done.ensure()
	if m.reconfigureEvents == nil {
		m.reconfigureEvents = make(chan struct{}, 1)
	}
}

func (m *Manager) closeManagerDoneLocked() {
	m.ensureLifecycleChannelsLocked()
	m.done.close()
}
