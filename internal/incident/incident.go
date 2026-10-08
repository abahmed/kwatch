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

// State flags of an Incident: who sets each, who clears it, what it means.
// The delivery-facing ones live in Delivery and Pending (flags.go) and
// change only through their methods.
//
//	Delivery.paged        MarkPaged on any own message; ClosePage on a
//	                      resolve sent to pagers. Alert open at pagers.
//	Delivery.pageHeld     HoldAtNotify (holdRepeatedPage, reopen); never
//	                      cleared. A repeated page notifies instead.
//	Delivery.rolledUp     MarkRolledUp; never cleared. A roll-up carried
//	                      it, so more failing pods are news.
//	Delivery.sentTier,
//	sentGrowth            RecordSent in decide, replaced by the next one:
//	                      what people were last told.
//	Delivery.lastMaterial MarkMaterial; spaces material updates.
//	Pending.reopenedAt    ScheduleReopenUpdate (reopen); ClearReopen
//	                      (reopenUpdate, resolve). "failing again" is due.
//	Pending.revised       MarkRevised (cause revision); ClearRevised
//	                      (revisedStep). Say "cause revised" once.
//	decided               set by decide, replaced by the next one.
//	refingerprint (Manager): restored incidents adopting a fingerprint.

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
	// Checked lists the kinds of objects upstream of the failures that
	// were reached and found healthy and unchanged when no cause was
	// found: "node", "image", "configmap". A message says so instead of
	// leaving the reader with no cause at all. Sorted, without repeats.
	Checked []string
	// Considered lists the other causes the solver weighed for the
	// latest placement, best first, as "root (row, confidence)". The
	// audit log carries them; messages do not.
	Considered []string
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
	// Attempt is the latest change to the root's workload, or to a
	// ConfigMap or Secret it uses, made while the incident was open:
	// someone trying to fix it. Nil when none. It is not persisted.
	Attempt *inventory.Change
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
	// AnnouncedRoute is how the first announcement was routed to
	// providers; nil until then, and for incidents announced by an older
	// kwatch. Persisted. Delivery routes the resolve with it (see
	// route.go).
	AnnouncedRoute *AnnouncedRoute
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
	// SupersededRoot is the root of that incident, so the closing message
	// can name it. It is not persisted: the message is written at once.
	SupersededRoot inventory.EntityID
	// Reminded is when the last weekly "still open" update was sent;
	// zero until the first.
	Reminded time.Time
	// DigestedAt is when a digest last listed this incident; zero when
	// none did. A recurrence reads it through History (see
	// DigestWorthy).
	DigestedAt time.Time
	// Owner is who the incident belongs to, as of the latest decision;
	// see owner.go. Routing reads it. Not persisted: AnnouncedRoute is.
	Owner string
	// Ack is the acknowledgement on the root or a member, nil when there
	// is none (see ack.go). Persisted.
	Ack *Ack
	// Delivery is what people were told and where it reached (see
	// flags.go). Pending is follow-up work the thread is owed.
	Delivery Delivery
	Pending  Pending

	// decided is the set of failure findings at the latest decision, so
	// a later re-blaming of the same failures can be told from news.
	// It is replaced, never changed, so snapshots may share it. Not
	// persisted: without it nothing is damped.
	decided map[detection.Key]struct{}
	// RepeatCount is how many times, this one included, the incident came
	// back within RepageWindow at its latest reopen; zero when it never
	// reopened. Writers read it instead of the timeline note.
	RepeatCount int
	// attemptLate records that the thread already said the incident
	// still fails FixWatch after Attempt.
	attemptLate bool
	// attemptSeen is a fix attempt observed but not yet told. Apply
	// records it when the change arrives; attemptUpdate tells it on the
	// first tick no earlier step ends, then moves it to Attempt. Not
	// persisted: the change history still holds the change.
	attemptSeen *inventory.Change
	// movedTo lists the roots members left for since people last heard
	// about the incident. When they all went to one place the story
	// moved there; when they dispersed, the story is over.
	movedTo []inventory.EntityID
	// formerRoot is the root the incident was moved away from by its
	// latest cause revision, and rerootedAt is when; see retakeFormer.
	formerRoot inventory.EntityID
	rerootedAt time.Time
	// rootReasons are the root's own finding reasons ever seen in this
	// incident. The fingerprint reads them, so a crash loop whose pods
	// come up and fall over again, toggling the workload's own
	// conditions, is not a stream of "material changes".
	rootReasons map[string]struct{}
	// causeFindings are the "entity=reason" pairs of the cause's root
	// findings ever seen. The fingerprint reads them, so a finding that
	// ages out of the cause is not a "material change". Not persisted:
	// a restored incident adopts its fingerprint.
	causeFindings map[string]struct{}
	// impactPeak is the largest impact size seen. The fingerprint reads it,
	// so impact that shrinks while failures churn is not news.
	impactPeak int
	// modes is every failure mode the members of this incident had; see
	// modes.go.
	modes map[detection.Mode]struct{}
	// stagePeak is the worst stage the members reached; the fingerprint
	// reads it, so a crash loop that begins after the announcement is
	// one update (see worsen.go).
	stagePeak stage
	// persistent marks a failure that outlasted the boot window while a
	// member crash-looped or the workload had nothing ready: known and
	// rhythmic demotion no longer applies. maxedLong marks an autoscaler
	// at its maximum for HPAStuckAfter. Neither is persisted; the next
	// tick judges them again.
	persistent, maxedLong bool
	// causeChain records that the cause blames another object of the
	// root's own workload chain (a pod, a Service, a ReplicaSet). The
	// reasoning flips between those for the very same failures, so the
	// fingerprint does not read which one.
	causeChain bool
	// bootedAt is when this run first ticked a restored incident; see
	// bootStart. Zero for incidents opened by this run.
	bootedAt time.Time
	// quiet marks an incident that resolveQuietly closed without a
	// message. Not persisted: the announcer reads it within a tick.
	quiet bool
	// updateHeld marks a material-change update that is waiting for its
	// investigation, and prevDigest is the fingerprint before it. While
	// the update is held the record keeps prevDigest, so a restart that
	// loses the held update still sees the change as news.
	updateHeld bool
	prevDigest string
	// restored marks an incident loaded from the state file; only such
	// incidents wait out the restore grace.
	restored bool
	// trafficLost records that a Service in the impact, routed to by an
	// Ingress or route, has no ready backends or mostly failing ones.
	// It is recomputed with the impact; the traffic-lost page reads it.
	trafficLost bool
	// servingDown records that a user-facing workload in the root or
	// impact has no ready replica. It is recomputed with the impact;
	// the last-replica-down page reads it.
	servingDown bool
	// admissionBlocked records that the root is a fail-closed admission
	// webhook whose failed calls block creates. Such a root often has
	// no finding of its own (its endpoints are ready, its backend just
	// does not answer), so its members alone are not critical. It is
	// recomputed with the impact; the admission-blocked page reads it.
	admissionBlocked bool
	// rejectionSeen records that a request was refused because of the
	// incident's webhooks (idleWebhook), and when it was last seen. The
	// time is persisted, so a restored page keeps what it earned.
	rejectionSeen bool
	rejectionAt   time.Time
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

// TrafficLost reports an incident that takes requests away from
// someone: a Service in its impact, routed to by an Ingress or route,
// has no ready backends or mostly failing ones. Digest lists put such
// incidents first.
func (inc Incident) TrafficLost() bool {
	return inc.trafficLost
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

// Event is one line of the incident timeline.
type Event struct {
	At   time.Time
	Text string
	// Entity is the member the event is about: the one that joined or
	// recovered. Nil for events about the incident itself.
	Entity *inventory.EntityID `json:",omitempty"`
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
	out.Checked = append([]string(nil), inc.Checked...)
	out.Considered = append([]string(nil), inc.Considered...)
	out.AnnouncedRoute = inc.AnnouncedRoute.clone()
	out.Ack = inc.Ack.clone()
	out.Reported, out.ReportedKnown = inc.sent.reported(len(inc.Timeline))
	if inc.Cause != nil {
		cause := *inc.Cause
		out.Cause = &cause
	}
	if inc.FixedBy != nil {
		fix := *inc.FixedBy
		out.FixedBy = &fix
	}
	if inc.Attempt != nil {
		attempt := *inc.Attempt
		out.Attempt = &attempt
	}
	return out
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
