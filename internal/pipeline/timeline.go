package pipeline

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// TimelineKind says what a timeline entry records. Values are stable:
// they are persisted.
type TimelineKind string

// Timeline entry kinds, in causal order: a change causes events, events
// feed findings, findings drive decisions.
const (
	TimelineChange   TimelineKind = "change"
	TimelineEvent    TimelineKind = "event"
	TimelineFinding  TimelineKind = "finding"
	TimelineDecision TimelineKind = "decision"
)

// TimelineSkew is the clock skew tolerated between the API server and
// kwatch. Entries closer together than this are ordered by cause, then
// by receive time, rather than by their timestamps; an API time further
// ahead of the receive time is treated as skew and replaced by it.
const TimelineSkew = 2 * time.Second

// TimelineEntry is one timestamped fact about an entity: a health
// transition, a change, a Kubernetes Warning event or a kwatch decision.
type TimelineEntry struct {
	// At is the API server time when known, else the receive time.
	At time.Time
	// Received is when kwatch learned of it.
	Received time.Time
	Kind     TimelineKind
	// Entity is the entity key the entry belongs to.
	Entity string
	// Action is raised, changed or cleared for findings, and announce,
	// update or resolve for decisions.
	Action string `json:",omitempty"`
	Reason string `json:",omitempty"`
	Mode   string `json:",omitempty"`
	Health string `json:",omitempty"`
	Text   string `json:",omitempty"`
	// Class and ChangeSet describe a change.
	Class     string `json:",omitempty"`
	ChangeSet string `json:",omitempty"`
	// FirstSeen and Count describe a repeated event.
	FirstSeen time.Time `json:",omitzero"`
	Count     int       `json:",omitempty"`
	// Incident is the incident a decision belongs to.
	Incident string `json:",omitempty"`

	// changeAt is the change's own time, which finds its change set even
	// when At was clamped for skew. It is not persisted.
	changeAt time.Time
}

// timelineTime clamps an API time that is ahead of the receive time by
// more than the skew tolerance.
func timelineTime(apiTime, received time.Time) time.Time {
	if apiTime.IsZero() || apiTime.After(received.Add(TimelineSkew)) {
		return received
	}
	return apiTime
}

// kindRank is the causal order of kinds within the skew tolerance.
func kindRank(kind TimelineKind) int {
	switch kind {
	case TimelineChange:
		return 0
	case TimelineEvent:
		return 1
	case TimelineFinding:
		return 2
	}
	return 3
}

// SortTimeline orders entries by API server time. Entries within
// TimelineSkew of their neighbour keep causal order (change, event,
// finding, decision) and then receive order, because a two-second clock
// difference says nothing about which happened first.
func SortTimeline(entries []TimelineEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].At.Before(entries[j].At)
	})
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && causallyBefore(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// causallyBefore reports whether a, which sorts after b by time, belongs
// before it because both lie within the skew tolerance.
func causallyBefore(a, b TimelineEntry) bool {
	if a.At.Sub(b.At) > TimelineSkew {
		return false
	}
	if kindRank(a.Kind) != kindRank(b.Kind) {
		return kindRank(a.Kind) < kindRank(b.Kind)
	}
	return a.Received.Before(b.Received)
}

// changeEntry records a change. The change set is filled in by the
// writer, off the decision loop.
func changeEntry(change inventory.Change) TimelineEntry {
	received := change.ReceivedAt()
	return TimelineEntry{
		At: timelineTime(change.At, received), Received: received,
		Kind: TimelineChange, Entity: change.Entity.String(),
		Class: string(change.Classify()), Text: changeText(change),
		changeAt: change.At,
	}
}

func changeText(change inventory.Change) string {
	text := string(change.Classify())
	if change.Actor != "" {
		text += " by " + change.Actor
	}
	if change.Revision != "" {
		text += ", revision " + change.Revision
	}
	return text
}

// eventEntry records a Kubernetes Warning event.
func eventEntry(
	id inventory.EntityID, note inventory.Note, received time.Time,
) TimelineEntry {
	return TimelineEntry{
		At: timelineTime(note.At, received), Received: received,
		Kind: TimelineEvent, Entity: id.String(), Reason: note.Reason,
		Text: note.Message, FirstSeen: note.FirstSeen, Count: note.Count,
	}
}

var transitionActions = map[detection.TransitionKind]string{
	detection.Raised:  "raised",
	detection.Changed: "changed",
	detection.Cleared: "cleared",
}

// findingEntry records a finding transition. Findings carry their own
// start time, which is only used for a raise.
func findingEntry(t detection.Transition, now time.Time) TimelineEntry {
	s := t.Finding
	at := now
	if t.Kind == detection.Raised {
		at = timelineTime(s.Since, now)
	}
	return TimelineEntry{
		At: at, Received: now, Kind: TimelineFinding,
		Entity: s.Entity.String(), Action: transitionActions[t.Kind],
		Reason: s.Reason, Mode: string(s.Mode), Health: s.Health.String(),
		Text: s.Summary,
	}
}

var decisionActions = map[incident.Action]string{
	incident.Announce: "announce",
	incident.Update:   "update",
	incident.Resolve:  "resolve",
}

// decisionEntry records a kwatch decision on the incident's root.
func decisionEntry(d incident.Decision, now time.Time) TimelineEntry {
	return TimelineEntry{
		At: now, Received: now, Kind: TimelineDecision,
		Entity: d.Incident.Root.String(), Action: decisionActions[d.Action],
		Text: d.Reason, Incident: d.Incident.ID,
		Mode: string(d.Incident.Mode),
	}
}
