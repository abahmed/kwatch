package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Flip-back damping. A crash loop looks different at each look: the
// container runs, ends with Error, then waits in CrashLoopBackOff, and
// the reasoning may blame the workload, then something it depends on,
// then the workload again, each time for the very same failures. Each
// swing closes one incident as superseded and, with the next, opens a
// new one, with a new first message and a new page, for a failure
// people were told about long ago.

// retakeSuperseded sends a failure that came back to the root of a
// superseded incident to the incident that took over from it. The
// failure is the same story when its object is part of the old
// incident's workload, and the story continues while the new incident
// is open and the old one ended less than RepageWindow ago. A failure
// after RepageWindow is news and gets an incident of its own.
func (m *Manager) retakeSuperseded(
	now time.Time, s detection.Finding, where placement,
) placement {
	old := m.lookup(where.root)
	if old == nil || old.State != Resolved || old.SupersededBy == "" ||
		now.Sub(old.Resolved) > RepageWindow || !m.ofStory(old, s) {
		return where
	}
	return m.stayWith(m.incidents[old.SupersededBy], where)
}

// retakeFormer does the same when the incident was not superseded but
// moved: its root changed with the revised cause, and the failure now
// goes back to the root it left. It stays under the root it moved to,
// while that incident is open and the move was less than RepageWindow
// ago.
func (m *Manager) retakeFormer(
	now time.Time, s detection.Finding, where placement,
) placement {
	id, ok := m.former[where.root.String()]
	if !ok {
		return where
	}
	moved := m.incidents[id]
	if moved == nil || moved.formerRoot != where.root ||
		now.Sub(moved.rerootedAt) > RepageWindow {
		delete(m.former, where.root.String())
		return where
	}
	if held := m.lookup(where.root); (held != nil &&
		held.State != Resolved) || !m.ofStory(moved, s) {
		return where
	}
	return m.stayWith(moved, where)
}

// stayWith places the failure in target, if it is announced, keeping
// the cause target has: the reasoning that just flipped back would only
// overwrite it with a lapse.
func (m *Manager) stayWith(
	target *Incident, where placement,
) placement {
	if target == nil || !wasAnnounced(target) {
		return where
	}
	where.root, where.cause, where.unclear = target.Root, nil, false
	return where
}

// leaveRoot remembers that p is moving away from its root, and drops
// the memories of moves too old to matter.
func (m *Manager) leaveRoot(now time.Time, p *Incident) {
	for root, id := range m.former {
		if moved := m.incidents[id]; moved == nil ||
			now.Sub(moved.rerootedAt) > RepageWindow {
			delete(m.former, root)
		}
	}
	p.formerRoot, p.rerootedAt = p.Root, now
	m.former[p.Root.String()] = p.ID
}

// restoreFormer rebuilds the memory of the move of a restored incident.
// A root left by two incidents remembers the later move.
func (m *Manager) restoreFormer(p *Incident) {
	if p.formerRoot == (inventory.EntityID{}) {
		return
	}
	key := p.formerRoot.String()
	if old := m.incidents[m.former[key]]; old != nil &&
		old.rerootedAt.After(p.rerootedAt) {
		return
	}
	m.former[key] = p.ID
}

// ofStory reports whether the object of s is part of the story p
// tells: p's root is its workload, or the Service in front of it. The
// timeline cannot say, as it keeps only the latest events.
func (m *Manager) ofStory(p *Incident, s detection.Finding) bool {
	return m.model != nil && sameChain(m.model, p.Root, s.Entity)
}

// dropStaleSupersede cancels the supersede of p when the incident that
// took over has ended itself since. Closing p into an incident that is
// already resolved would say "moved there" about a story nobody tells.
// p goes on as it is: its members come back, or it recovers on its own.
func (m *Manager) dropStaleSupersede(p *Incident) {
	if p.SupersededBy == "" {
		return
	}
	if target := m.incidents[p.SupersededBy]; target == nil ||
		target.State == Resolved {
		p.SupersededBy, p.SupersededRoot = "", inventory.EntityID{}
	}
}
