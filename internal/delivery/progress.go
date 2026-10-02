package delivery

import (
	"context"
	"time"
)

func (m *Manager) nowTime() time.Time {
	return m.now()
}

// LastProgress implements the application lifecycle progress contract.
//
// A worker in the middle of a job is making progress even when it waits a
// long time: pacing, waiting out a provider's outage or Retry-After, or
// sending a slow request. Such waits are bounded by their own deadlines,
// so while any worker is busy delivery reports progress now; only a
// worker that stops taking jobs at all can stall the component.
func (m *Manager) LastProgress() time.Time {
	if m.busy.Load() > 0 {
		return m.nowTime()
	}
	value := m.lastProgress.Load()
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}

func (m *Manager) touchProgress() {
	m.lastProgress.Store(m.nowTime().UnixNano())
}

// markBusy marks a worker busy with a job until the returned func runs.
func (m *Manager) markBusy() func() {
	m.busy.Add(1)
	return func() {
		m.busy.Add(-1)
		m.touchProgress()
	}
}

// requestContext is the context provider requests use: the send context
// while delivery runs, ctx otherwise.
func (m *Manager) requestContext(ctx context.Context) context.Context {
	m.mu.Lock()
	send := m.workers.sendCtx
	m.mu.Unlock()
	if send == nil || send.Err() != nil {
		return ctx
	}
	return send
}
