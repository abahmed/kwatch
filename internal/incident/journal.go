package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// The journal is what the dashboard shows of an incident's life beyond
// its messages. Nothing in it decides an alert: no fingerprint, tier or
// timer reads it, so it can grow fields without changing what anyone
// is told. Every list in it is bounded, and all of it but Timers is
// persisted with the record.

// Journal is the incident's own record of its life.
type Journal struct {
	// Breaks are the failing containers when the incident first broke
	// and when it last got worse, oldest first; at most maxBreaks.
	Breaks []BreakSnapshot
	// Changes are the changes people made around the incident, copied
	// from the change history while it was live so they outlive it.
	// Field values are bounded and Secret and ConfigMap data names its
	// keys only (see SafeChange). At most maxLinkedChanges.
	Changes []inventory.Change
	// Transitions are the incident's state and tier changes, oldest
	// first; at most maxTransitions.
	Transitions []Transition
	// ErrorKey is what "the same error" means for this incident (see
	// SameErrorKey), stored when it was announced; empty before.
	ErrorKey string
	// Level is the code of why the incident is at its tier (see
	// AlertLevelText); empty when not known yet.
	Level string
	// Widest is the widest the incident reached (see radius.go): what
	// a resolve says was affected after every member left.
	Widest Radius
	// Timers are what the incident waited on at the latest tick. They
	// are not persisted: the next tick computes them again.
	Timers Timers
	// lookedAt is when the changes and the first break were last
	// looked for; seenState and seenTier are the state and tier the
	// latest transition left, so the next one can be told apart.
	lookedAt  time.Time
	seenState State
	seenTier  Tier
}

// Transition is one change of an incident's state or tier. From and To
// are state names ("open") for a state change and tier names ("page")
// for a tier change.
type Transition struct {
	At   time.Time
	Kind string
	From string `json:",omitempty"`
	To   string `json:",omitempty"`
}

// Transition kinds. Values are stable: they are persisted and served.
const (
	TransitionOpened       = "opened"
	TransitionAnnounced    = "announced"
	TransitionRecovering   = "recovering"
	TransitionFailingAgain = "failing again"
	TransitionFlapping     = "flapping"
	TransitionReopened     = "reopened"
	TransitionResolved     = "resolved"
	TransitionStateChanged = "state changed"
	TransitionTierRaised   = "tier raised"
	TransitionTierLowered  = "tier lowered"
)

// Journal bounds.
const (
	maxTransitions = 50
	// lookEvery spaces the looks for changes and for a first break of a
	// live incident: each reads the change history or the pods.
	lookEvery = time.Minute
)

// clone copies the journal's lists, so a snapshot shares none of them.
// Stored changes and break snapshots are replaced whole, never edited,
// so their own slices are shared.
func (j Journal) clone() Journal {
	out := j
	out.Breaks = append([]BreakSnapshot(nil), j.Breaks...)
	out.Changes = append([]inventory.Change(nil), j.Changes...)
	out.Transitions = append([]Transition(nil), j.Transitions...)
	return out
}

// SameErrorKey is what "the same error" means: the failure mode (the
// stable identity of the root's finding, kept across resolves and
// restarts) and, when kwatch found one, the rule of the likely cause.
// With neither there is no key, and nothing is the same error.
func SameErrorKey(mode detection.Mode, rule string) string {
	if mode == "" && rule == "" {
		return ""
	}
	return string(mode) + "\x00" + rule
}

// ErrorKey is the stored same-error key of the incident, or, for an
// incident not announced yet or saved by an older kwatch, the key of
// its mode and cause now.
func (inc Incident) ErrorKey() string {
	if inc.Journal.ErrorKey != "" {
		return inc.Journal.ErrorKey
	}
	rule := ""
	if inc.Cause != nil {
		rule = inc.Cause.Rule
	}
	return SameErrorKey(inc.Mode, rule)
}

// observe records what changed about p since it was last observed: its
// transitions, why it is at its tier, a snapshot when it broke or got
// worse, its error key and the changes around it; with NoJournalDetail
// neither the snapshot nor the changes. Apply observes the incidents it
// changed and Tick every live one. It decides nothing.
func (m *Manager) observe(p *Incident, now time.Time) {
	j := &p.Journal
	if p.State != Resolved && (j.Level == "" || j.seenTier != p.Tier) {
		p.noteLevel(m.levelOf(p))
	}
	worse := j.noteTransitions(p, now, m.tierUnsettled(p, now))
	if p.State == Resolved {
		return
	}
	if p.State != Settling && j.ErrorKey == "" {
		j.ErrorKey = p.ErrorKey()
	}
	if m.cfg.NoJournalDetail {
		return
	}
	if worse != "" {
		m.takeBreak(p, now, worse)
	}
	if p.Tier == Silent || now.Sub(j.lookedAt) < lookEvery {
		return
	}
	j.lookedAt = now
	if len(j.Breaks) == 0 {
		m.takeBreak(p, now, BreakFirst)
	}
	m.linkChanges(p, now)
}

// noteTransitions appends p's state and tier changes since the last
// observation and returns why a snapshot is due: BreakFirst when it
// opened, BreakWorse when its tier rose or it reopened, else "".
func (j *Journal) noteTransitions(
	p *Incident, now time.Time, unsettled bool,
) string {
	worse := ""
	if from := j.seenState; from != p.State {
		kind := transitionKind(from, p.State)
		j.add(Transition{At: now, Kind: kind, From: stateName(from),
			To: p.State.String()})
		switch kind {
		case TransitionOpened:
			worse = BreakFirst
		case TransitionReopened:
			worse = BreakWorse
		}
	}
	if unsettled {
		// The tier of a restored incident moves with a partial view
		// until the grace is over: only where it lands is recorded.
		j.seenState = p.State
		return worse
	}
	if j.seenTier != p.Tier && j.seenState != 0 &&
		p.State != Settling && p.State != Resolved {
		kind := TransitionTierLowered
		if p.Tier > j.seenTier {
			kind, worse = TransitionTierRaised, BreakWorse
		}
		j.add(Transition{At: now, Kind: kind, From: j.seenTier.String(),
			To: p.Tier.String()})
	}
	j.seenState, j.seenTier = p.State, p.Tier
	return worse
}

// add appends t, keeping the newest maxTransitions.
func (j *Journal) add(t Transition) {
	j.Transitions = append(j.Transitions, t)
	if over := len(j.Transitions) - maxTransitions; over > 0 {
		j.Transitions = append(j.Transitions[:0:0],
			j.Transitions[over:]...)
	}
}

// transitionKind names the move from one state to another.
func transitionKind(from, to State) string {
	switch {
	case from == 0:
		return TransitionOpened
	case to == Resolved:
		return TransitionResolved
	case from == Resolved:
		return TransitionReopened
	case from == Settling && to == Open:
		return TransitionAnnounced
	case to == Recovering:
		return TransitionRecovering
	case to == Flapping:
		return TransitionFlapping
	case from == Recovering && to == Open:
		return TransitionFailingAgain
	}
	return TransitionStateChanged
}

// stateName names a state, or "" for none.
func stateName(s State) string {
	if s == 0 {
		return ""
	}
	return s.String()
}

// restoreJournal rebuilds the journal from a record: what was last
// observed is the restored state, so a restart is no transition.
func restoreJournal(p *Incident, r Record) {
	p.Journal = Journal{
		Breaks: r.Breaks, Changes: r.LinkedChanges,
		Transitions: r.Transitions, ErrorKey: r.ErrorKey,
		Level: r.Level, Widest: r.Widest,
		seenState: p.State, seenTier: p.Tier,
	}
}
