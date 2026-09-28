package signal

import (
	"sort"
	"sync"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// TransitionKind says what happened to a signal.
type TransitionKind uint8

// Transition kinds.
const (
	Raised TransitionKind = iota + 1
	Changed
	Cleared
)

// Transition is one change in the set of active signals.
type Transition struct {
	Kind   TransitionKind
	Signal Signal
}

// Tracker holds the active signals and turns evaluations into
// transitions. Changed is reported only when the severity or summary
// differs, never for identical re-evaluations.
type Tracker struct {
	mu     sync.Mutex
	active map[knowledge.EntityID]map[string]Signal
}

// NewTracker builds an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{active: make(map[knowledge.EntityID]map[string]Signal)}
}

// Observe replaces the entity's active signals with current and returns
// the transitions in a deterministic order.
func (t *Tracker) Observe(
	id knowledge.EntityID, current []Signal,
) []Transition {
	t.mu.Lock()
	defer t.mu.Unlock()
	previous := t.active[id]
	next := make(map[string]Signal, len(current))
	var out []Transition
	for _, s := range current {
		old, existed := previous[s.Reason]
		if existed {
			s.Since = old.Since
		}
		next[s.Reason] = s
		switch {
		case !existed:
			out = append(out, Transition{Kind: Raised, Signal: s})
		case old.Severity != s.Severity || old.Summary != s.Summary:
			out = append(out, Transition{Kind: Changed, Signal: s})
		}
	}
	for reason, s := range previous {
		if _, still := next[reason]; !still {
			out = append(out, Transition{Kind: Cleared, Signal: s})
		}
	}
	if len(next) == 0 {
		delete(t.active, id)
	} else {
		t.active[id] = next
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Signal.Reason < out[j].Signal.Reason
	})
	return out
}

// Active returns the entity's current signals.
func (t *Tracker) Active(id knowledge.EntityID) []Signal {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Signal, 0, len(t.active[id]))
	for _, s := range t.active[id] {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}

// Has reports whether the entity has any active signal.
func (t *Tracker) Has(id knowledge.EntityID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.active[id]) > 0
}
