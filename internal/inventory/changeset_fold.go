package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"sync"
	"time"
)

// setCache holds the model's change sets. Sets are built incrementally:
// each new change joins the sets it is linked to, or starts a new one, and
// sets that did not change are left alone. That keeps a set's ID and
// membership stable while the rest of the cluster keeps changing.
//
// It has its own lock because readers fold new changes in while holding
// only the model's read lock; writers never run at the same time.
type setCache struct {
	mu sync.Mutex
	// folded is the highest change sequence folded into sets.
	folded uint64
	// evictions is the history eviction count last applied.
	evictions uint64
	sets      []*liveSet
	// view is the published, ordered snapshot of sets.
	view  []ChangeSet
	stale bool
}

// liveSet is one change set under construction.
type liveSet struct {
	set   ChangeSet
	seqs  []uint64
	dirty bool
}

func newSetCache() *setCache { return &setCache{} }

// changeSetsLocked returns the current sets, oldest first, after folding
// in changes that arrived since the last call. The caller holds at least
// the model's read lock.
func (m *Model) changeSetsLocked() []ChangeSet {
	cache := m.history.cache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.evictions != m.history.evictions {
		cache.dropEvicted(m.history.liveSeqs())
		cache.evictions = m.history.evictions
	}
	if cache.folded != m.history.seq {
		scopeOf := m.scopeCache()
		for _, entry := range m.history.entriesAfter(cache.folded) {
			cache.fold(entry, scopeOf)
		}
		cache.folded = m.history.seq
	}
	if cache.stale {
		cache.publish()
	}
	return cache.view
}

// scopeCache memoises scopeLocked for one fold.
func (m *Model) scopeCache() func(EntityID) entityScope {
	scopes := make(map[EntityID]entityScope)
	return func(id EntityID) entityScope {
		if scope, ok := scopes[id]; ok {
			return scope
		}
		scope := m.scopeLocked(id)
		scopes[id] = scope
		return scope
	}
}

// fold adds one change. It joins every set that has a linked member at
// most ChangeSetWindow away; when it joins several, they merge into the
// oldest, which keeps its ID.
func (c *setCache) fold(
	entry changeEntry, scopeOf func(EntityID) entityScope,
) {
	var joined []*liveSet
	for _, live := range c.sets {
		if live.linksTo(entry.change, scopeOf) {
			joined = append(joined, live)
		}
	}
	c.stale = true
	if len(joined) == 0 {
		c.sets = append(c.sets, &liveSet{
			set: ChangeSet{
				ID:    c.mintID(entry.change),
				Start: entry.change.At, End: entry.change.At,
				Changes: []Change{entry.change},
			},
			seqs: []uint64{entry.seq}, dirty: true,
		})
		return
	}
	target := oldestSet(joined)
	for _, other := range joined {
		if other != target && target.mergeFits(other, entry.change.At) {
			target.absorb(other)
		}
	}
	target.add(entry)
	c.removeEmpty()
}

// mergeFits reports whether absorbing other, and the change made at at,
// keeps the set within ChangeSetMaxSpan. A change that links two sets
// must not bridge them into one longer than the cap: the other set stays
// apart and the change joins this one.
func (s *liveSet) mergeFits(other *liveSet, at time.Time) bool {
	start, end := s.set.Start, s.set.End
	for _, t := range []time.Time{other.set.Start, at} {
		if t.Before(start) {
			start = t
		}
	}
	for _, t := range []time.Time{other.set.End, at} {
		if t.After(end) {
			end = t
		}
	}
	return end.Sub(start) <= ChangeSetMaxSpan
}

// ChangeSetMaxSpan caps how long one change set can grow. Without it a
// chatty app or a controller that never stops changing things would chain
// every change into one ever-growing "release".
const ChangeSetMaxSpan = 15 * time.Minute

func (s *liveSet) linksTo(
	change Change, scopeOf func(EntityID) entityScope,
) bool {
	// Cheap checks on the set's whole span first, so the members are only
	// scanned for sets the change can possibly join.
	start, end := s.set.Start, s.set.End
	if change.At.Before(start.Add(-ChangeSetWindow)) ||
		change.At.After(end.Add(ChangeSetWindow)) {
		return false
	}
	if change.At.After(end) {
		end = change.At
	}
	if change.At.Before(start) {
		start = change.At
	}
	if end.Sub(start) > ChangeSetMaxSpan {
		return false
	}
	for _, member := range s.set.Changes {
		gap := change.At.Sub(member.At)
		if gap < 0 {
			gap = -gap
		}
		if gap <= ChangeSetWindow && linked(member, change, scopeOf) {
			return true
		}
	}
	return false
}

// add appends a member. Start and End are kept current right away,
// because merges in the same fold compare them.
func (s *liveSet) add(entry changeEntry) {
	s.set.Changes = append(s.set.Changes, entry.change)
	s.seqs = append(s.seqs, entry.seq)
	s.widen(entry.change.At, entry.change.At)
}

// absorb moves other's members into s and leaves other empty. s keeps
// other's ID, and the IDs other had absorbed, as aliases.
func (s *liveSet) absorb(other *liveSet) {
	s.set.Aliases = append(s.set.Aliases, other.set.ID)
	s.set.Aliases = append(s.set.Aliases, other.set.Aliases...)
	s.set.Changes = append(s.set.Changes, other.set.Changes...)
	s.seqs = append(s.seqs, other.seqs...)
	s.widen(other.set.Start, other.set.End)
	other.set.Changes, other.seqs = nil, nil
}

func (s *liveSet) widen(start, end time.Time) {
	if start.Before(s.set.Start) {
		s.set.Start = start
	}
	if end.After(s.set.End) {
		s.set.End = end
	}
	s.dirty = true
}

// oldestSet picks the set created first: the earliest start, then the
// smaller ID, so a merge is deterministic.
func oldestSet(sets []*liveSet) *liveSet {
	oldest := sets[0]
	for _, live := range sets[1:] {
		if live.set.Start.Before(oldest.set.Start) ||
			(live.set.Start.Equal(oldest.set.Start) &&
				live.set.ID < oldest.set.ID) {
			oldest = live
		}
	}
	return oldest
}

// dropEvicted removes members that left the history. A set keeps its ID
// even when its first change goes.
func (c *setCache) dropEvicted(live map[uint64]bool) {
	for _, s := range c.sets {
		kept, seqs := s.set.Changes[:0], s.seqs[:0]
		for i, change := range s.set.Changes {
			if live[s.seqs[i]] {
				kept = append(kept, change)
				seqs = append(seqs, s.seqs[i])
			}
		}
		if len(kept) != len(s.set.Changes) {
			s.dirty = true
			c.stale = true
		}
		s.set.Changes, s.seqs = kept, seqs
	}
	c.removeEmpty()
}

func (c *setCache) removeEmpty() {
	kept := c.sets[:0]
	for _, s := range c.sets {
		if len(s.set.Changes) > 0 {
			kept = append(kept, s)
		}
	}
	clear(c.sets[len(kept):])
	c.sets = kept
}

// mintID derives a new set's ID from its first change, so the same
// history gives the same IDs after a restart. A clash with a live set
// gets a numeric suffix.
func (c *setCache) mintID(first Change) string {
	sum := sha256.Sum256([]byte(first.Entity.String() + "@" +
		strconv.FormatInt(first.At.UnixNano(), 10)))
	base := "cs-" + hex.EncodeToString(sum[:5])
	id := base
	for n := 2; c.hasID(id); n++ {
		id = base + "-" + strconv.Itoa(n)
	}
	return id
}

// hasID reports whether a live set has id as its ID or as an alias, so
// a new set never takes an ID that an older timeline entry resolves.
func (c *setCache) hasID(id string) bool {
	for _, s := range c.sets {
		if s.set.ID == id || containsText(s.set.Aliases, id) {
			return true
		}
	}
	return false
}

// publish refreshes changed sets and rebuilds the ordered snapshot.
func (c *setCache) publish() {
	for _, s := range c.sets {
		if s.dirty {
			s.refresh()
		}
	}
	sort.SliceStable(c.sets, func(i, j int) bool {
		a, b := c.sets[i].set, c.sets[j].set
		if !a.Start.Equal(b.Start) {
			return a.Start.Before(b.Start)
		}
		return a.ID < b.ID
	})
	// The view gets its own member slices: later folds reorder and trim
	// the live ones in place.
	c.view = make([]ChangeSet, len(c.sets))
	for i, s := range c.sets {
		c.view[i] = s.set
		c.view[i].Changes = append([]Change(nil), s.set.Changes...)
		c.view[i].Aliases = append([]string(nil), s.set.Aliases...)
	}
	c.stale = false
}

// refresh orders members oldest first and recomputes the summary fields.
// The ID is never recomputed.
func (s *liveSet) refresh() {
	sort.Sort(membersByTime{s})
	changes := s.set.Changes
	s.set.Start = changes[0].At
	s.set.End = changes[len(changes)-1].At
	s.set.Who = setWho(changes)
	s.set.Label = setLabel(s.set)
	s.dirty = false
}

// membersByTime sorts a set's changes by time, then arrival.
type membersByTime struct{ s *liveSet }

func (m membersByTime) Len() int { return len(m.s.seqs) }
func (m membersByTime) Less(i, j int) bool {
	a, b := m.s.set.Changes[i].At, m.s.set.Changes[j].At
	if !a.Equal(b) {
		return a.Before(b)
	}
	return m.s.seqs[i] < m.s.seqs[j]
}
func (m membersByTime) Swap(i, j int) {
	c := m.s.set.Changes
	c[i], c[j] = c[j], c[i]
	m.s.seqs[i], m.s.seqs[j] = m.s.seqs[j], m.s.seqs[i]
}
