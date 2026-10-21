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

// observeServerTimeLocked folds the signed server time of a verified lease
// into the guard. Normally that only moves the mark forward. When the mark
// is more than rollbackTolerance ahead of the server, the local clock
// stepped forward and was corrected, so a lease newer than the cached one
// (a replay of an old response cannot be) pulls the mark back to the
// server's time and Pro recovers at once instead of when real time catches
// up. The caller holds m.mu.
func (m *Manager) observeServerTimeLocked(lease Lease) {
	srv := time.Unix(lease.SrvTime, 0).UTC()
	if m.clock.MaxSeen.After(srv.Add(rollbackTolerance)) &&
		m.newerThanCachedLocked(lease) {
		m.clock.MaxSeen = srv
	}
	m.clock.Observe(srv)
}

// newerThanCachedLocked reports a lease issued after the cached one, or
// any lease when none is cached.
func (m *Manager) newerThanCachedLocked(lease Lease) bool {
	iat, ok := leaseIssuedAt(m.lease)
	return !ok || lease.Iat > iat
}

// signedLeaseTime is the latest time a cached lease proves has passed:
// its signed iat or srv_time. Only a verified signature counts; the
// binding is not checked, since the time is real for any cluster.
func (v *Verifier) signedLeaseTime(s string) (time.Time, bool) {
	l, ok := v.signedLease(s)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(max(l.Iat, l.SrvTime), 0).UTC(), true
}
