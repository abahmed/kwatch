package selfcheck

import (
	"slices"
	"sync"
	"time"
)

// MaxMisses bounds the likely misses kept; the oldest go first.
const MaxMisses = 50

// Miss is one resolved incident whose recovery followed a change to
// something other than the cause it named (OtherChanged): the cause it
// named was likely wrong. It is kept for review, never acted on.
type Miss struct {
	// ID is the incident.
	ID string `json:"id"`
	// Rule and Root are the cause it named: the rule and the entity
	// blamed, as "kind/namespace/name".
	Rule string `json:"rule"`
	Root string `json:"root"`
	// Other is what changed instead, as "kind/namespace/name", and
	// Change what kind of change it was ("image", "config").
	Other  string `json:"other,omitempty"`
	Change string `json:"change,omitempty"`
	// Opened and Resolved bound the incident.
	Opened   time.Time `json:"opened"`
	Resolved time.Time `json:"resolved"`
}

// Misses holds the likely misses of the last Window days, at most
// MaxMisses. It is safe for concurrent use.
type Misses struct {
	mu    sync.Mutex
	items []Miss
}

// Record keeps m, replacing an earlier miss of the same incident.
func (s *Misses) Record(m Miss) {
	if m.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = slices.DeleteFunc(s.items, func(old Miss) bool {
		return old.ID == m.ID
	})
	s.items = append(s.items, m)
	s.pruneLocked(m.Resolved)
}

// pruneLocked drops misses resolved before the window and the oldest
// past MaxMisses.
func (s *Misses) pruneLocked(now time.Time) {
	start := windowStart(now)
	s.items = slices.DeleteFunc(s.items, func(m Miss) bool {
		return m.Resolved.Before(start)
	})
	slices.SortStableFunc(s.items, func(a, b Miss) int {
		return a.Resolved.Compare(b.Resolved)
	})
	if over := len(s.items) - MaxMisses; over > 0 {
		s.items = slices.Clone(s.items[over:])
	}
}

// List returns the misses of the window that ends at now, newest first.
func (s *Misses) List(now time.Time) []Miss {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := windowStart(now)
	var out []Miss
	for i := len(s.items) - 1; i >= 0; i-- {
		if !s.items[i].Resolved.Before(start) {
			out = append(out, s.items[i])
		}
	}
	return out
}

// Snapshot returns a copy of every miss held, oldest first, for saving.
func (s *Misses) Snapshot() []Miss {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.items)
}

// Restore replaces the misses with saved ones, bounded as Record bounds
// them (the window ends at the newest one), so a damaged value cannot
// grow the list. List drops what is older than its own window.
func (s *Misses) Restore(saved []Miss) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
	var newest time.Time
	for _, m := range saved {
		if m.ID != "" && !m.Resolved.IsZero() {
			s.items = append(s.items, m)
			newest = latestTime(newest, m.Resolved)
		}
	}
	s.pruneLocked(newest)
}

func latestTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
