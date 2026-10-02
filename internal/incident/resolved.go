package incident

import (
	"sort"
	"time"
)

// Bounds on resolved incidents. A resolved incident is kept only so a
// recurrence can link to it; its history is copied into the recurrence,
// so a few per root are enough. The total bound protects memory and the
// persisted state from a storm of short blips across many roots.
const (
	maxResolvedPerRoot = 3
	maxResolved        = 2000
)

// resolvedIndex orders resolved incidents by when they resolved, oldest
// first, so expiry and trimming read the front instead of sorting every
// incident on every tick. order may hold IDs already forgotten or
// resolved again later; readers skip them, and compact drops them once
// they outnumber the live entries.
type resolvedIndex struct {
	order  []string
	byRoot map[string][]string
	// count is the number of IDs in byRoot, the resolved incidents kept.
	count int
}

func newResolvedIndex() resolvedIndex {
	return resolvedIndex{byRoot: map[string][]string{}}
}

// minCompactOrder is the order length below which compact never runs,
// so a handful of stale IDs does not cause a rebuild on every resolve.
const minCompactOrder = 64

// add appends id, resolved just now, to root's list and to the order.
func (r *resolvedIndex) add(id, root string) {
	r.order = append(r.order, id)
	r.byRoot[root] = append(r.byRoot[root], id)
	r.count++
	r.compact()
}

// compact rebuilds order from the kept IDs when stale entries make up
// more than half of it. A flapping root resolves, recurs and resolves
// again, leaving one stale ID per cycle; without this the order would
// grow until the oldest kept incident expires, up to a week later.
// Only the last occurrence of an ID is kept: that is when it resolved.
func (r *resolvedIndex) compact() {
	if len(r.order) < minCompactOrder || len(r.order) <= 2*r.count {
		return
	}
	kept := make(map[string]bool, r.count)
	for _, ids := range r.byRoot {
		for _, id := range ids {
			kept[id] = true
		}
	}
	out := make([]string, 0, r.count)
	for i := len(r.order) - 1; i >= 0; i-- {
		if id := r.order[i]; kept[id] {
			out = append(out, id)
			delete(kept, id)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	r.order = out
}

// markResolved records p, which has just resolved, and enforces the
// per-root and total bounds, forgetting the oldest first.
func (m *Manager) markResolved(p *Incident) {
	root := p.Root.String()
	m.resolved.add(p.ID, root)
	for len(m.resolved.byRoot[root]) > maxResolvedPerRoot {
		m.forgetID(m.resolved.byRoot[root][0])
	}
	for m.resolvedCount() > maxResolved {
		m.forgetID(m.oldestResolved().ID)
	}
}

// resolvedCount is the number of resolved incidents still kept.
func (m *Manager) resolvedCount() int {
	return m.resolved.count
}

// oldestResolved returns the resolved incident that resolved first,
// dropping forgotten IDs from the front of the order. Nil when none.
func (m *Manager) oldestResolved() *Incident {
	for len(m.resolved.order) > 0 {
		p := m.incidents[m.resolved.order[0]]
		if p != nil && p.State == Resolved {
			return p
		}
		m.resolved.order = m.resolved.order[1:]
	}
	return nil
}

// expireResolved forgets resolved incidents older than Remember.
func (m *Manager) expireResolved(now time.Time) {
	for {
		p := m.oldestResolved()
		if p == nil || now.Sub(p.Resolved) <= m.cfg.Remember {
			return
		}
		m.forget(p)
	}
}

// forgetID forgets the incident with id, if it is still kept.
func (m *Manager) forgetID(id string) {
	if p := m.incidents[id]; p != nil {
		m.forget(p)
		return
	}
	m.dropResolved(id, "")
}

// forget drops an incident, its root index entry and its place in the
// resolved index.
func (m *Manager) forget(p *Incident) {
	delete(m.incidents, p.ID)
	m.unindex(p)
	m.dropResolved(p.ID, p.Root.String())
}

// dropResolved removes id from its root's resolved list. The order list
// is cleaned lazily by oldestResolved. An empty root searches all roots.
func (m *Manager) dropResolved(id, root string) {
	for key, ids := range m.resolved.byRoot {
		if root != "" && key != root {
			continue
		}
		for i, held := range ids {
			if held != id {
				continue
			}
			ids = append(ids[:i:i], ids[i+1:]...)
			m.resolved.count--
			if len(ids) == 0 {
				delete(m.resolved.byRoot, key)
			} else {
				m.resolved.byRoot[key] = ids
			}
			return
		}
	}
}

// indexResolvedRestored rebuilds the resolved index after a restore, in
// resolve order.
func (m *Manager) indexResolvedRestored() {
	m.resolved = newResolvedIndex()
	var done []*Incident
	for _, p := range m.incidents {
		if p.State == Resolved {
			done = append(done, p)
		}
	}
	sort.Slice(done, func(i, j int) bool {
		if !done[i].Resolved.Equal(done[j].Resolved) {
			return done[i].Resolved.Before(done[j].Resolved)
		}
		return done[i].ID < done[j].ID
	})
	for _, p := range done {
		m.markResolved(p)
	}
}

// liveIDs returns the IDs of incidents that are not resolved, sorted so
// decisions come out in a stable order.
func (m *Manager) liveIDs() []string {
	var ids []string
	for id, p := range m.incidents {
		if p.State != Resolved {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
