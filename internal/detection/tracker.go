package detection

import (
	"sort"
	"sync"

	"github.com/abahmed/kwatch/internal/inventory"
)

// TransitionKind says what happened to a finding.
type TransitionKind uint8

// Transition kinds.
const (
	Raised TransitionKind = iota + 1
	Changed
	Cleared
)

// Transition is one change in the set of active findings.
type Transition struct {
	Kind    TransitionKind
	Finding Finding
}

// Tracker holds the active findings and turns evaluations into
// transitions. Changed is reported only when the severity, health or
// mode differs; summaries carry elapsed times and counts, so text alone
// never re-announces a steady incident. The latest finding, with its
// current summary, is always kept.
type Tracker struct {
	mu     sync.Mutex
	active map[inventory.EntityID]map[string]Finding
}

// NewTracker builds an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{active: make(map[inventory.EntityID]map[string]Finding)}
}

// Observe replaces the entity's active findings with current and returns
// the transitions in a deterministic order.
func (t *Tracker) Observe(
	id inventory.EntityID, current []Finding,
) []Transition {
	t.mu.Lock()
	defer t.mu.Unlock()
	previous := t.active[id]
	next := make(map[string]Finding, len(current))
	var out []Transition
	for _, s := range current {
		s = Classify(s)
		old, existed := previous[s.Reason]
		if existed {
			s.Since = old.Since
		}
		next[s.Reason] = s
		switch {
		case !existed:
			out = append(out, Transition{Kind: Raised, Finding: s})
		case changed(old, s):
			out = append(out, Transition{Kind: Changed, Finding: s})
		}
	}
	for reason, s := range previous {
		if _, still := next[reason]; !still {
			out = append(out, Transition{Kind: Cleared, Finding: s})
		}
	}
	if len(next) == 0 {
		delete(t.active, id)
	} else {
		t.active[id] = next
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Finding.Reason < out[j].Finding.Reason
	})
	return out
}

// Active returns the entity's current findings.
func (t *Tracker) Active(id inventory.EntityID) []Finding {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Finding, 0, len(t.active[id]))
	for _, s := range t.active[id] {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}

// Has reports whether the entity has any active finding.
func (t *Tracker) Has(id inventory.EntityID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.active[id]) > 0
}

// changed reports whether a re-evaluated finding describes a different
// condition than the one already announced.
func changed(old, current Finding) bool {
	return old.Severity != current.Severity ||
		old.Health != current.Health || old.Mode != current.Mode
}
