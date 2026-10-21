package license

import (
	"context"
	"time"
)

// resyncMinGap is the least time between two resyncs that check-in
// verdicts request, so a server that keeps refusing cannot drive the
// renew loop.
const resyncMinGap = 5 * time.Minute

// ResyncOnVerdict wakes the renew loop at once when err is a check-in
// answer that ends Pro (expired, revoked or suspended), so the license
// follows the server within one check-in instead of at the next daily
// renewal. The renewal applies the verdict; the check-in answer itself is
// never trusted for the state. It reports whether a resync was requested.
func (m *Manager) ResyncOnVerdict(err error) bool {
	switch verdictFor(err) {
	case VerdictExpired, VerdictRevoked, VerdictSuspended:
	default:
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.cfg.Now()
	if !m.lastResync.IsZero() && now.Sub(m.lastResync) < resyncMinGap {
		return false
	}
	m.lastResync = now
	if m.wakeCancel != nil {
		m.woken = true
		m.wakeCancel()
	} else {
		m.wakePending = true
	}
	return true
}

// sleep waits like Config.Sleep but ends early, without an error, when
// ResyncOnVerdict asks for a resync.
func (m *Manager) sleep(ctx context.Context, d time.Duration) error {
	m.mu.Lock()
	if m.wakePending {
		m.wakePending = false
		m.mu.Unlock()
		return ctx.Err()
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.wakeCancel, m.woken = cancel, false
	m.mu.Unlock()
	err := m.cfg.Sleep(sctx, d)
	m.mu.Lock()
	woken := m.woken
	m.wakeCancel, m.woken = nil, false
	m.mu.Unlock()
	if woken && ctx.Err() == nil {
		return nil
	}
	return err
}
