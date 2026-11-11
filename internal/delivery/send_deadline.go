package delivery

import (
	"context"
	"sync"
	"time"
)

// Every provider request runs under a deadline, so a provider that never
// answers cannot hold a worker for good. The transport has its own,
// shorter HTTP timeout (30 seconds); this one is the backstop for a
// provider call that has none.
const (
	// sendTimeout is the longest one provider request may take. It must
	// stay at or above the transport's HTTP timeout.
	sendTimeout = 2 * time.Minute
	// sendStallGrace is how long past its deadline a request may still
	// run before it stops counting as progress: a provider that honours
	// its context has returned by then.
	sendStallGrace = 30 * time.Second
)

// sendTracker remembers when each provider request in flight began.
type sendTracker struct {
	mu     sync.Mutex
	seq    uint64
	starts map[uint64]time.Time
}

func (t *sendTracker) begin(now time.Time) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.starts == nil {
		t.starts = make(map[uint64]time.Time)
	}
	t.seq++
	t.starts[t.seq] = now
	return t.seq
}

func (t *sendTracker) end(id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.starts, id)
}

// oldest is when the longest-running request began; false when none runs.
func (t *sendTracker) oldest() (time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var first time.Time
	for _, start := range t.starts {
		if first.IsZero() || start.Before(first) {
			first = start
		}
	}
	return first, !first.IsZero()
}

// beginSend returns the context for one provider request, with the send
// deadline, and registers the request for the stall check. The caller
// runs done when the request returns.
func (m *Manager) beginSend(
	ctx context.Context,
) (context.Context, func()) {
	base := m.requestContext(ctx)
	limit := m.sendDeadline()
	requestCtx, cancel := context.WithTimeout(base, limit)
	id := m.sends.begin(m.nowTime())
	return requestCtx, func() {
		m.sends.end(id)
		cancel()
	}
}

func (m *Manager) sendDeadline() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.workers.sendLimit > 0 {
		return m.workers.sendLimit
	}
	return sendTimeout
}

// stalledSendSince is when the oldest request in flight passed its
// deadline plus grace without returning; false when none did. Such a
// request ignores its context: a busy worker is no longer progress.
func (m *Manager) stalledSendSince() (time.Time, bool) {
	start, ok := m.sends.oldest()
	if !ok {
		return time.Time{}, false
	}
	limit := m.sendDeadline()
	if m.nowTime().Sub(start) <= limit+sendStallGrace {
		return time.Time{}, false
	}
	return start.Add(limit), true
}
