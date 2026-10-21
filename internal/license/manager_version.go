package license

import "k8s.io/klog/v2"

// VersionStore is an optional LeaseStore extension that remembers the
// kwatch version last reported to the server by a successful activation
// or renewal. A LeaseStore without it keeps the plain renew schedule.
type VersionStore interface {
	LoadReportedVersion() (string, error)
	SaveReportedVersion(version string) error
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

// recordReportedLocked persists the version just accepted by the server.
func (m *Manager) recordReportedLocked() {
	vs, ok := m.cfg.LeaseStore.(VersionStore)
	if !ok || m.cfg.Version == "" ||
		(m.reportedKnown && m.reported == m.cfg.Version) {
		return
	}
	if err := vs.SaveReportedVersion(m.cfg.Version); err != nil {
		klog.ErrorS(err, "persist reported kwatch version",
			"component", "license")
		return
	}
	m.reported, m.reportedKnown = m.cfg.Version, true
}
