package license

import "time"

// rollbackTolerance is how far the wall clock may fall behind the latest
// observed time before it is treated as rolled back.
const rollbackTolerance = time.Hour

// ClockGuard remembers the latest time seen so a rolled-back system clock
// cannot extend a lease.
type ClockGuard struct {
	MaxSeen time.Time
}

// Observe records t if it is later than anything seen so far.
func (c *ClockGuard) Observe(t time.Time) {
	if t.After(c.MaxSeen) {
		c.MaxSeen = t
	}
}

// RolledBack reports whether now is more than an hour before MaxSeen.
func (c ClockGuard) RolledBack(now time.Time) bool {
	return now.Before(c.MaxSeen.Add(-rollbackTolerance))
}

// Effective returns the later of now and MaxSeen.
func (c ClockGuard) Effective(now time.Time) time.Time {
	if c.MaxSeen.After(now) {
		return c.MaxSeen
	}
	return now
}

// observeLocalLocked folds the local reading into the guard so that every
// refresh and status tick moves the high-water mark, and writes it out at
// least hourly. now.Sub uses the monotonic reading when it has one, so a
// wall clock that was set back or is frozen still falls behind and is
// caught by RolledBack. The caller holds m.mu.
func (m *Manager) observeLocalLocked(now time.Time) {
	if m.anchor.IsZero() {
		m.anchor = now
	}
	m.clock.Observe(now)
	m.clock.Observe(m.anchor.Round(0).Add(now.Sub(m.anchor)))
	if m.clock.MaxSeen.Sub(m.savedMax) >= clockPersistEvery {
		m.persistLocked()
	}
}
