package problem

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

// State is a problem's lifecycle stage.
type State uint8

// Lifecycle states.
const (
	// Settling problems are collecting related signals before the first
	// message.
	Settling State = iota + 1
	// Open problems have been announced and are still failing.
	Open
	// Recovering problems have no active signals and wait for the hold.
	Recovering
	// Flapping problems keep failing and recovering; transitions are
	// silent until the pattern changes or it becomes stable.
	Flapping
	// Resolved problems are closed.
	Resolved
)

// Tier is how loudly a problem is delivered.
type Tier uint8

// Tiers from quietest to loudest.
const (
	Silent Tier = iota
	Digest
	Notify
	Page
)

// Problem is one root cause and everything it explains.
type Problem struct {
	ID    string
	Root  knowledge.EntityID
	Cause *reason.Hypothesis
	// Members are the signals this problem explains, keyed by signal key.
	Members map[signal.Key]signal.Signal
	Impact  []knowledge.EntityID
	Tier    Tier
	State   State

	Opened          time.Time
	Announced       time.Time
	RecoveringSince time.Time
	Resolved        time.Time
	// Cycles records when the problem recovered, for flap detection.
	Cycles []time.Time
	// Occurrences records when the problem opened, across resolves, so a
	// routine (same time every day) is recognised.
	Occurrences []time.Time

	// Revision counts announced messages; Digest fingerprints the last
	// announced content so unchanged content is never re-sent.
	Revision int
	Digest   string
	Timeline []Event
}

// Event is one line of the problem timeline.
type Event struct {
	At   time.Time
	Text string
}

// Snapshot returns a detached copy safe to hand to writers.
func (p *Problem) Snapshot() Problem {
	out := *p
	out.Members = make(map[signal.Key]signal.Signal, len(p.Members))
	for key, s := range p.Members {
		out.Members[key] = s
	}
	out.Impact = append([]knowledge.EntityID(nil), p.Impact...)
	out.Cycles = append([]time.Time(nil), p.Cycles...)
	out.Occurrences = append([]time.Time(nil), p.Occurrences...)
	out.Timeline = append([]Event(nil), p.Timeline...)
	if p.Cause != nil {
		cause := *p.Cause
		out.Cause = &cause
	}
	return out
}

// Action is what a decision asks delivery to do.
type Action uint8

// Actions.
const (
	Announce Action = iota + 1
	Update
	Resolve
)

// Decision is one message-worthy transition.
type Decision struct {
	Action  Action
	Problem Problem
	// Reason explains why this decision was made, for the audit trail.
	Reason string
	// Output holds application output gathered by investigation after the
	// decision, already redacted. Empty when none was needed or found.
	Output []string
}

// String names the state for diagnostics.
func (s State) String() string {
	switch s {
	case Settling:
		return "settling"
	case Open:
		return "open"
	case Recovering:
		return "recovering"
	case Flapping:
		return "flapping"
	case Resolved:
		return "resolved"
	}
	return "unknown"
}

// String names the tier for diagnostics.
func (t Tier) String() string {
	switch t {
	case Silent:
		return "silent"
	case Digest:
		return "digest"
	case Notify:
		return "notify"
	case Page:
		return "page"
	}
	return "unknown"
}
