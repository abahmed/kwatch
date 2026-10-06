package incident

// Reason says why a decision was made. Renderers and the audit trail
// switch on the declared values; resolve reasons also carry a duration
// ("healthy for 5m0s"), which is why this is a string type.
type Reason string

// Decision reasons that renderers and audits distinguish. Add every new
// one to AllReasons: a compose test fails until compose handles it.
const (
	// ReasonSettled is the announcement of an incident that finished
	// collecting its first findings.
	ReasonSettled Reason = "settled"
	// ReasonCauseRevised is an update whose incident moved to a new root.
	ReasonCauseRevised Reason = "cause revised"
	// ReasonMaterialChange is an update because the incident's
	// fingerprint changed: new members, a new impact.
	ReasonMaterialChange Reason = "material change"
	// ReasonFlapping is the update that says an open incident keeps
	// failing and recovering.
	ReasonFlapping Reason = "flapping"
	// ReasonSuperseded closes an incident whose members now belong to
	// another announced incident.
	ReasonSuperseded Reason = "superseded by revised cause"
)

// ReasonRecoveredUnheard closes the pager alert of an incident that paged
// while its announcement was held, and recovered before it was restored
// and settled again. Like the "healthy for" reasons it is not in
// AllReasons: the message it makes is a paging-only resolve.
const ReasonRecoveredUnheard Reason = "recovered before it was announced"

// StoppedTrackingPrefix starts the resolve reason of an incident that
// ended at its StillBrokenMax cap while its workload was still short
// of replicas. The reason names the cap and says the coverage check
// carries on, since "healthy" would be false (see stoppedTracking).
const StoppedTrackingPrefix = "stopped tracking after "

// AllReasons lists every fixed reason, for tests that check a consumer
// handles each one. Resolve reasons that carry a duration are not fixed.
var AllReasons = []Reason{
	ReasonSettled, ReasonCauseRevised, ReasonMaterialChange,
	ReasonFlapping, ReasonSuperseded, ReasonReminder,
	ReasonFixAttempt, ReasonFixStillFailing, ReasonFailingAgain,
}

// RecoveredPrefix starts the timeline note of a member that recovered;
// composers read it to tell recoveries from new failures.
const RecoveredPrefix = "recovered: "
