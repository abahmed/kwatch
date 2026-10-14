package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// outlivedEviction reports an evicted pod that no cause explains now and
// whose workload runs all the replicas it wants. The eviction happened
// once, the replacement runs, and the failed pod only stays listed until
// the cluster collects it, as it does after the node pressure that
// caused it has cleared. It is history, not a failure that exists now.
func (m *Manager) outlivedEviction(
	s detection.Finding, where placement,
) bool {
	if s.Reason != reasons.Evicted || where.cause != nil || m.model == nil {
		return false
	}
	desired, ready, ok := ReplicasOf(m.model, where.root)
	return ok && desired > 0 && ready >= desired
}

// leaveHistory takes a finding out of its incident, if it is in one: it
// is over, though its object is still listed.
func (m *Manager) leaveHistory(now time.Time, s detection.Finding) {
	if id, ok := m.byMember[s.Key()]; ok {
		m.changed[id] = true
	}
	m.detach(now, s)
}
