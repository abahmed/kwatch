package detection

import (
	"sort"
	"sync"

	"github.com/abahmed/kwatch/internal/format"
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
// transitions. Changed is reported only when the severity, health, mode
// or error line differs; summaries carry elapsed times and counts, so
// text alone never re-announces a steady incident. The latest finding,
// with its current summary, is always kept.
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
// condition than the one already announced. A new or different error
// line counts: the crash log is read after the finding is raised, and
// the incident must then be regrouped by the message it carries.
func changed(old, current Finding) bool {
	return old.Severity != current.Severity ||
		old.Health != current.Health || old.Mode != current.Mode ||
		errorChanged(errorShape(old), errorShape(current))
}

// errorChanged reports a new or different error. A line that goes away
// is not announced: the crash log is cleared between restarts.
func errorChanged(old, current string) bool {
	return current != "" && old != current
}

// errorShape is the error line in its normalised form (format.Signature):
// times, counts, addresses and request ids are replaced. A line that only
// counts ("not renewed for 3m5s") or carries a fresh address or id on
// every restart is the same error, so it does not announce a change on
// every evaluation. A line that appears, or says something else, does.
func errorShape(f Finding) string {
	return format.Signature(evidenceValue(f, EvidenceError))
}

// evidenceValue returns the first evidence value with the label, or "".
func evidenceValue(f Finding, label string) string {
	for _, e := range f.Evidence {
		if e.Label == label {
			return e.Value
		}
	}
	return ""
}
