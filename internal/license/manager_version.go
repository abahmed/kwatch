package license

import (
	"time"

	"k8s.io/klog/v2"
)

// Early renewals for one new version are spaced by an exponential window
// so a crash loop with the server unreachable cannot hammer it.
const (
	earlyRenewBase = time.Minute
	earlyRenewCap  = 30 * time.Minute
)

// RenewAttempt is an early renewal started for a new kwatch version that
// has not succeeded yet. Its zero value means none is pending.
type RenewAttempt struct {
	Version string    `json:"version"`
	At      time.Time `json:"at"`
	Count   int       `json:"count"`
}

// VersionStore is an optional LeaseStore extension that remembers the
// kwatch version last reported to the server by a successful activation
// or renewal. A LeaseStore without it keeps the plain renew schedule.
type VersionStore interface {
	LoadReportedVersion() (string, error)
	SaveReportedVersion(version string) error
	LoadRenewAttempt() (RenewAttempt, error)
	SaveRenewAttempt(a RenewAttempt) error
}

// window is how long after this attempt another early one may start.
func (a RenewAttempt) window() time.Duration {
	d := earlyRenewBase
	for i := 1; i < a.Count && d < earlyRenewCap; i++ {
		d *= 2
	}
	return min(d, earlyRenewCap)
}

// loadReportedLocked reads the recorded version. A read error leaves the
// version unknown, which never forces an early renewal: a store that keeps
// failing must not turn every restart into a server call.
func (m *Manager) loadReportedLocked() {
	vs, ok := m.cfg.LeaseStore.(VersionStore)
	if !ok {
		return
	}
	v, err := vs.LoadReportedVersion()
	if err != nil {
		klog.ErrorS(err, "load reported kwatch version",
			"component", "license")
		return
	}
	m.reported, m.reportedKnown = v, true
}

// versionChanged reports whether this kwatch version differs from the one
// the server last accepted, so the first renewal should not wait. No
// recorded version (the first run that keeps one) counts as a change.
func (m *Manager) versionChanged() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reportedKnown && m.cfg.Version != "" &&
		m.reported != m.cfg.Version
}

// earlyRenewDelay returns 0 and persists the attempt when an early renew
// may start now. While an earlier attempt for this version is inside its
// window it returns the time left, so restarts do not add calls.
func (m *Manager) earlyRenewDelay() time.Duration {
	vs, ok := m.cfg.LeaseStore.(VersionStore)
	if !ok {
		return 0
	}
	now := m.effective()
	last, err := vs.LoadRenewAttempt()
	if err != nil {
		klog.ErrorS(err, "load license renew attempt", "component", "license")
		return renewInterval // unknown: keep the plain schedule
	}
	next := RenewAttempt{Version: m.cfg.Version, At: now, Count: 1}
	if last.Version == m.cfg.Version {
		if left := last.At.Add(last.window()).Sub(now); left > 0 {
			return left
		}
		next.Count = last.Count + 1
	}
	if err := vs.SaveRenewAttempt(next); err != nil {
		klog.ErrorS(err, "persist license renew attempt",
			"component", "license")
		return renewInterval // unrecorded attempts could repeat per restart
	}
	return 0
}

// recordReportedLocked persists the version just accepted by the server.
func (m *Manager) recordReportedLocked() {
	vs, ok := m.cfg.LeaseStore.(VersionStore)
	if !ok || m.cfg.Version == "" ||
		(m.reportedKnown && m.reported == m.cfg.Version) {
		return
	}
	if err := vs.SaveRenewAttempt(RenewAttempt{}); err != nil {
		klog.ErrorS(err, "clear license renew attempt",
			"component", "license")
	}
	if err := vs.SaveReportedVersion(m.cfg.Version); err != nil {
		klog.ErrorS(err, "persist reported kwatch version",
			"component", "license")
		return
	}
	m.reported, m.reportedKnown = m.cfg.Version, true
}
