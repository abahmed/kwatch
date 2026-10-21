package incident

import (
	"sort"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/safego"
)

// Tick advances every incident's lifecycle and returns the decisions that
// warrant a message, plus the delay until the earliest pending deadline
// computed from the state after all transitions (zero when none).
func (m *Manager) Tick(now time.Time) ([]Decision, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var decisions []Decision
	m.adoptFingerprints(now)
	m.releaseChurn(now)
	// Only live incidents advance; resolved ones only wait to expire.
	for _, id := range m.tickOrder() {
		if d, ok := m.tickOne(id, now); ok {
			decisions = append(decisions, d)
		}
	}
	m.expireResolved(now)
	m.pruneFormer(now)
	return decisions, m.nextWake(now)
}

// tickOrder lists the live incidents by ID, those that lost their
// failures to another incident last: the incident that took the failures
// over speaks before the ones it closes (see takeoverPending).
func (m *Manager) tickOrder() []string {
	ids := m.liveIDs()
	sort.SliceStable(ids, func(i, j int) bool {
		return m.incidents[ids[i]].SupersededBy == "" &&
			m.incidents[ids[j]].SupersededBy != ""
	})
	return ids
}

// tickOne advances one live incident and reports its decision. A panic
// is contained here, per incident: that incident keeps what it had until
// the next tick, and the decisions already made for the others stand. A
// bad object then costs one incident one tick, not every message of the
// tick.
func (m *Manager) tickOne(id string, now time.Time) (d Decision, ok bool) {
	p := m.incidents[id]
	if p == nil {
		// An earlier incident of this tick merged it (see reroot).
		return Decision{}, false
	}
	err := safego.Run("incident", func() error {
		if p.restored && p.bootedAt.IsZero() {
			p.bootedAt = now
		}
		d, ok = m.advance(p, now)
		if p.State == Resolved && m.incidents[id] == p {
			m.markResolved(p)
		}
		return nil
	})
	if err != nil {
		klog.ErrorS(err, "incident: tick of one incident failed",
			"component", "incident", "operation", "tick", "incident", id)
		return Decision{}, false
	}
	return d, ok
}
