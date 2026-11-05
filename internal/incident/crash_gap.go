package incident

import "time"

// A crash loop is quiet between two crashes, and a container without a
// readiness probe even looks ready while it runs. Its finding clears
// and comes back with the next crash: that is no recovery, and its
// return is no flap cycle. A recovery is a gap of the crash loop when
// the workload's containers were crashing (in back-off, or a run that
// ended with an error within the hold) while the incident recovered.

// noteCrashGap marks p while it recovers and its workload crashes.
func (m *Manager) noteCrashGap(p *Incident, now time.Time) {
	if p.State != Recovering || !m.crashing(p, now) {
		return
	}
	if m.gaps == nil {
		m.gaps = map[string]bool{}
	}
	m.gaps[p.ID] = true
}

// crashGap reports a recovering incident whose members came back after
// a gap of its crash loop, and forgets the mark once its members are
// back or it resolves.
func (m *Manager) crashGap(p *Incident, now time.Time) bool {
	if len(p.Members) == 0 {
		m.noteCrashGap(p, now)
		return false
	}
	gap := m.gaps[p.ID]
	delete(m.gaps, p.ID)
	return gap
}

// CrashLoopGap is how long a crash loop may run between two crashes:
// the kubelet resets a container's back-off once it has run this long,
// so a shorter run is part of the loop, not a recovery.
const CrashLoopGap = 10 * time.Minute

// crashLoopReturn reports a failure of p back within CrashLoopGap of its
// end while its workload's containers crash: the quiet between was a
// run of the crash loop, so the return counts no flap cycle, whether it
// reopens, revives or recurs.
func (m *Manager) crashLoopReturn(p *Incident, end, now time.Time) bool {
	return !end.IsZero() && now.Sub(end) < CrashLoopGap &&
		m.crashing(p, now)
}
