package license

import (
	"time"

	"k8s.io/klog/v2"
)

// While the license is bound to another cluster the manager asks again
// often, because the owner may release it any minute: every five minutes
// for the first hour of a conflict, then hourly.
const (
	conflictFastEvery = 5 * time.Minute
	conflictFastFor   = time.Hour
	conflictSlowEvery = time.Hour
)

// conflictDelay returns the delay before the next activation attempt in a
// conflict. It logs once, when the conflict starts.
func (m *Manager) conflictDelay() time.Duration {
	now := m.cfg.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures = 0
	if m.conflictSince.IsZero() {
		m.conflictSince = now
		klog.InfoS("license is bound to another cluster; retrying "+
			"every few minutes until it is released or moved here",
			"component", "license")
	}
	if now.Sub(m.conflictSince) < conflictFastFor {
		return conflictFastEvery
	}
	return conflictSlowEvery
}

// clearConflict ends a conflict streak and logs that it ended.
func (m *Manager) clearConflict() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conflictSince.IsZero() {
		return
	}
	m.conflictSince = time.Time{}
	klog.InfoS("license is no longer in conflict", "component", "license")
}
