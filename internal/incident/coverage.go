package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// CoveredWorkloads lists the workloads that a live incident with failing
// members speaks for, at any tier. A digest or silent incident is quiet
// on purpose (boot noise, a known routine), so it still covers: handing
// its workload back would only repeat the same decision every Retry.
// The one exception is a digest incident that must speak now: past the
// boot window the workload crash-loops or has nothing ready, and tier
// would raise it once persistent (see escalationOwed). That one is not
// covered, because escalation failed to do its job.
//
// A covering incident speaks for its root and the owners of its failing
// members. Unless it is quiet it also speaks for its impact: a digest
// item that lists a workload in its impact says nothing of its crash.
// The coverage check (pipeline/coverage/doc.go) compares the list with
// the workloads that fail to find one whose incident was lost.
func (m *Manager) CoveredWorkloads(
	now time.Time,
) map[inventory.EntityID]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	covered := map[inventory.EntityID]bool{}
	if m.model == nil {
		return covered
	}
	for _, p := range m.incidents {
		if p.State == Resolved || !hasFailingMember(p) ||
			m.escalationOwed(p, now) {
			continue
		}
		covered[workloadFor(m.model, p.Root)] = true
		for key, s := range p.Members {
			if !s.Advisory {
				covered[workloadFor(m.model, key.Entity)] = true
			}
		}
		if p.Tier < Notify {
			continue
		}
		for _, id := range p.Impact {
			covered[id] = true
		}
	}
	return covered
}

// escalationOwed reports a digest-tier incident that tier would raise
// now: it lasted past the boot window with a crash loop or nothing
// ready, and the tier it would have as a persistent incident is higher
// than the digest. escalate normally raises it in the same Tick.
func (m *Manager) escalationOwed(p *Incident, now time.Time) bool {
	if p.Tier != Digest || !(p.persistent || m.lastsPastBoot(p, now)) {
		return false
	}
	persistent := *p
	persistent.persistent = true
	return m.override.apply(&persistent, tier(&persistent)) > Digest
}

// RestoredFailing returns the announced incidents restored from the
// previous session that fail again now, loudest first by ID order, for
// the startup listing: a restart announces none of them one by one, so
// without it nobody would be told they still fail. Digest-tier and
// out-of-scope incidents are left out, and so are those that have spoken
// for themselves since the restart: the listing would repeat their own
// message.
func (m *Manager) RestoredFailing() []Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Incident
	for _, id := range m.sortedIDs() {
		p := m.incidents[id]
		if p.restored && wasAnnounced(p) && hasFailingMember(p) &&
			p.Tier >= Notify && p.Scope != ScopeOut &&
			p.sent.covered == unknownMark {
			out = append(out, p.Snapshot())
		}
	}
	return out
}
