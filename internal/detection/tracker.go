package detection

import (
	"sort"
	"strings"
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
	// Refreshed means the wording changed (summary or evidence), not the
	// condition. It is never announced; holders of the finding only
	// replace their copy so later messages quote fresh evidence.
	Refreshed
)

// Transition is one change in the set of active findings.
type Transition struct {
	Kind    TransitionKind
	Finding Finding
}

// Tracker holds the active findings and turns evaluations into
// transitions. Changed is reported only when the severity, health, mode
// or error line differs; summaries carry elapsed times and counts, so
// text alone never re-announces a steady incident. Text that says
// something new, once counts, times and ids are normalised away, is
// reported as Refreshed. The latest finding is always kept.
type Tracker struct {
	mu     sync.Mutex
	active map[inventory.EntityID]map[string]tracked
}

// NewTracker builds an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{active: make(map[inventory.EntityID]map[string]tracked)}
}

// Observe replaces the entity's active findings with current and returns
// the transitions in a deterministic order.
func (t *Tracker) Observe(
	id inventory.EntityID, current []Finding,
) []Transition {
	t.mu.Lock()
	defer t.mu.Unlock()
	previous := t.active[id]
	next := make(map[string]tracked, len(current))
	var out []Transition
	for _, s := range current {
		s = Classify(s)
		old, existed := previous[s.Reason]
		if existed {
			s.Since = old.finding.Since
		}
		now := newTracked(s, old, existed)
		next[s.Reason] = now
		switch {
		case !existed:
			out = append(out, Transition{Kind: Raised, Finding: s})
		case changed(old, now):
			out = append(out, Transition{Kind: Changed, Finding: s})
		case reworded(old, now):
			out = append(out, Transition{Kind: Refreshed, Finding: s})
		}
	}
	for reason, s := range previous {
		if _, still := next[reason]; !still {
			out = append(out, Transition{Kind: Cleared, Finding: s.finding})
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
		out = append(out, s.finding)
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

// tracked is a finding with its normalised text. format.Signature is
// costly, so each text is normalised once, when the finding is stored,
// and again only when the raw text differs from the stored one.
type tracked struct {
	finding Finding
	summary string // Signature of the summary
	extra   string // evidenceShape: normalised evidence, no error line
	// errLine is the error line in its normalised form: times, counts,
	// addresses and request ids are replaced, so a line that only counts
	// or carries a fresh id on every restart is the same error.
	errLine string
}

// newTracked normalises f, reusing old's work for text that is unchanged.
func newTracked(f Finding, old tracked, hasOld bool) tracked {
	t := tracked{finding: f}
	if hasOld && old.finding.Summary == f.Summary {
		t.summary = old.summary
	} else {
		t.summary = format.Signature(f.Summary)
	}
	if hasOld && sameEvidence(old.finding, f) {
		t.extra = old.extra
	} else {
		t.extra = evidenceShape(f)
	}
	oldErr := evidenceValue(old.finding, EvidenceError)
	newErr := evidenceValue(f, EvidenceError)
	if hasOld && oldErr == newErr {
		t.errLine = old.errLine
	} else {
		t.errLine = format.Signature(newErr)
	}
	return t
}

// sameEvidence reports identical labels and values, error line aside.
func sameEvidence(a, b Finding) bool {
	x, y := withoutError(a.Evidence), withoutError(b.Evidence)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i].Label != y[i].Label || x[i].Value != y[i].Value {
			return false
		}
	}
	return true
}

// withoutError drops the error line. It copies only when there is one.
func withoutError(in []Evidence) []Evidence {
	for i, e := range in {
		if e.Label == EvidenceError {
			out := append([]Evidence(nil), in[:i]...)
			for _, r := range in[i+1:] {
				if r.Label != EvidenceError {
					out = append(out, r)
				}
			}
			return out
		}
	}
	return in
}

// changed reports whether a re-evaluated finding describes a different
// condition than the one already announced. A new or different error
// line counts: the crash log is read after the finding is raised, and
// the incident must then be regrouped by the message it carries.
func changed(old, current tracked) bool {
	o, c := old.finding, current.finding
	return o.Severity != c.Severity ||
		o.Health != c.Health || o.Mode != c.Mode ||
		o.Normal != c.Normal ||
		errorChanged(old.errLine, current.errLine)
}

// reworded reports whether the summary or the evidence other than the
// error line says something new. It compares normalised text, like
// the error line, so a growing count or a fresh address is not news. The
// error line is left to changed: a line that goes away must not wipe
// the one the incident still quotes.
func reworded(old, current tracked) bool {
	return old.summary != current.summary || old.extra != current.extra
}

// evidenceShape joins the normalised evidence without the error line.
func evidenceShape(f Finding) string {
	var out []string
	for _, e := range f.Evidence {
		if e.Label != EvidenceError {
			out = append(out, e.Label+"="+format.Signature(e.Value))
		}
	}
	return strings.Join(out, "\n")
}

// errorChanged reports a new or different error. A line that goes away
// is not announced: the crash log is cleared between restarts.
func errorChanged(old, current string) bool {
	return current != "" && old != current
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
