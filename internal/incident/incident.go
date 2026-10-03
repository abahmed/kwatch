package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// State is an incident's lifecycle stage.
type State uint8

// Lifecycle states.
const (
	// Settling incidents are collecting related findings before the first
	// message.
	Settling State = iota + 1
	// Open incidents have been announced and are still failing.
	Open
	// Recovering incidents have no active findings and wait for the hold.
	Recovering
	// Flapping incidents keep failing and recovering; transitions are
	// silent until the pattern changes or it becomes stable.
	Flapping
	// Resolved incidents are closed.
	Resolved
)

// Tier is how loudly an incident is delivered.
type Tier uint8

// Tiers from quietest to loudest.
const (
	Silent Tier = iota
	Digest
	Notify
	Page
)

// Incident is one root cause and everything it explains.
type Incident struct {
	// ID is opaque and never changes, so delivery keeps one conversation
	// per incident. It is not derived from Root: a revised cause moves
	// the incident to a new root under the same ID.
	ID   string
	Root inventory.EntityID
	// Previous is the ID of the resolved incident this one repeats: the
	// same root failing again.
	Previous string
	Cause    *rootcause.CauseRecord
	// CauseUnclear is set when no cause reached the confidence floor
	// although something outside the root's own workload was considered
	// and dropped: the failure could have an upstream cause kwatch could
	// not confirm. It stays set until a cause is found, so every message
	// says the cause is not clear. A root with nothing upstream to weigh
	// (a node that stopped reporting) is not unclear.
	CauseUnclear bool
	// Unverified names what root-cause rules could not check because
	// kwatch cannot see that kind ("secrets in billing"). It is kept for
	// the incident's life so every message states the gap.
	Unverified []string
	// Members are the findings this incident explains, keyed by finding key.
	Members map[detection.Key]detection.Finding
	Impact  []inventory.EntityID
	Tier    Tier
	State   State

	Opened          time.Time
	Announced       time.Time
	RecoveringSince time.Time
	Resolved        time.Time
	// Cycles records when the incident recovered, for flap detection.
	Cycles []time.Time
	// Occurrences records when the incident opened, across resolves, so a
	// routine (same time every day) is recognised.
	Occurrences []time.Time
	// Mode is the failure mode of the root's finding, such as "CrashLoop".
	Mode detection.Mode
	// Fix is how this incident ended, judged when it resolved.
	Fix Fix
	// FixedBy is the change that most likely fixed the incident, found
	// when it resolved. Nil when nothing changed. It is not persisted:
	// only the resolve message reads it.
	FixedBy *inventory.Change
	// History holds the earlier resolved occurrences of this root, oldest
	// first, with how each ended.
	History []Occurrence

	// Revision counts announced messages; Digest fingerprints the last
	// announced content so unchanged content is never re-sent. Revision
	// never decreases for one incident, across restarts.
	Revision int
	Digest   string
	// AlertKey is the stable identity paging systems deduplicate on. It
	// is derived from the root and the failure mode when the incident is
	// first announced, never from the ID's random nonce, so after a
	// store reset the same failing root finds, updates and resolves its
	// old open alert. It then stays fixed, even when the cause is
	// revised, so one alert follows the incident to its resolve.
	AlertKey string
	Timeline []Event
	// Reported is how many entries at the start of Timeline the previous
	// delivered message already covered. ReportedKnown is false when that
	// is unknown, for example after a restart. A decision carries the
	// values from before itself, so an update can name everything newer.
	// Snapshot sets both; they are not persisted.
	Reported      int
	ReportedKnown bool
	// sent tracks Reported by counting entries rather than by time, so
	// entries noted in the same second are never lost (see report.go).
	sent sentMark

	// Scope remembers whether the announcement reached people, so its
	// updates and resolve follow it even when no finding is left to judge.
	Scope Scope
	// Held marks an announcement collected for the startup summary and
	// not yet handed to delivery.
	Held bool
	// SupersededBy is the ID of the incident that took over every member
	// after a cause revision. The incident closes without claiming
	// recovery.
	SupersededBy string

	// revised asks the next update to say the cause was revised, once
	// the new cause held since revisedAt for the revise settle.
	revised   bool
	revisedAt time.Time
	// impactPeak is the largest impact size seen. The fingerprint reads it,
	// so impact that shrinks while failures churn is not news.
	impactPeak int
	// restored marks an incident loaded from the state file; only such
	// incidents wait out the restore grace.
	restored bool
	// trafficLost records that a Service in the impact, routed to by an
	// Ingress or route, has no ready backends or mostly failing ones.
	// It is recomputed with the impact; the traffic-lost page reads it.
	trafficLost bool
	// admissionBlocked records that the root is a fail-closed admission
	// webhook whose failed calls block creates. Such a root often has
	// no finding of its own (its endpoints are ready, its backend just
	// does not answer), so its members alone are not critical. It is
	// recomputed with the impact; the admission-blocked page reads it.
	admissionBlocked bool
	// routedMissing records that the root is a Service an Ingress or
	// route sends traffic to and that does not exist: every request
	// routed to it fails. A Gateway API route reports this through a
	// generic, non-critical condition, so its members alone are not
	// critical either. It is recomputed with the impact.
	routedMissing bool
}

// criticalRoot reports a root that takes something away from everyone
// by itself, whatever the severity of the findings it explains.
func (inc *Incident) criticalRoot() bool {
	return inc.admissionBlocked || inc.routedMissing
}

// Scope records the delivery scope of an incident's announcement.
type Scope uint8

// Scopes.
const (
	// ScopeUnknown means no announcement was judged yet.
	ScopeUnknown Scope = iota
	// ScopeIn means the announcement was delivered.
	ScopeIn
	// ScopeOut means the announcement was dropped as out of scope.
	ScopeOut
)

// Decision reasons that renderers and audits distinguish.
const (
	// ReasonCauseRevised is an update whose incident moved to a new root.
	ReasonCauseRevised = "cause revised"
	// ReasonSuperseded closes an incident whose members now belong to
	// another announced incident.
	ReasonSuperseded = "superseded by revised cause"
)

// Event is one line of the incident timeline.
type Event struct {
	At   time.Time
	Text string
}

// Snapshot returns a detached copy safe to hand to writers.
func (inc *Incident) Snapshot() Incident {
	out := *inc
	out.Members = make(map[detection.Key]detection.Finding, len(inc.Members))
	for key, s := range inc.Members {
		out.Members[key] = s
	}
	out.Impact = append([]inventory.EntityID(nil), inc.Impact...)
	out.Cycles = append([]time.Time(nil), inc.Cycles...)
	out.Occurrences = append([]time.Time(nil), inc.Occurrences...)
	out.History = append([]Occurrence(nil), inc.History...)
	out.Timeline = append([]Event(nil), inc.Timeline...)
	out.Unverified = append([]string(nil), inc.Unverified...)
	out.Reported, out.ReportedKnown = inc.sent.reported(len(inc.Timeline))
	if inc.Cause != nil {
		cause := *inc.Cause
		out.Cause = &cause
	}
	if inc.FixedBy != nil {
		fix := *inc.FixedBy
		out.FixedBy = &fix
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
	Action   Action
	Incident Incident
	// Reason explains why this decision was made, for the audit trail.
	Reason string
	// Output holds application output gathered by investigation after the
	// decision, already redacted. Empty when none was needed or found.
	Output []string
	// Evidence holds the facts investigation found, already redacted.
	// Writers quote them as proof. Empty when none were found.
	Evidence []Fact
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
