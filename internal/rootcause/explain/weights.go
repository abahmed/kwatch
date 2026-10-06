package explain

import "time"

// Every number the solver uses lives here, with the reason for its
// value. Weights are added to a row's prior; the sum is clamped to
// [0, 1] and read as the confidence.

// Confidence thresholds.
const (
	// ConfidenceFloor is the least confidence a cause needs to be
	// stated. Below it the area is reported as unknown. It sits just
	// under a coin flip so a bare prior never passes without evidence
	// once a contradicting point is found.
	ConfidenceFloor = 0.45
	// ConfidenceLikely is where a cause is worded as likely.
	ConfidenceLikely = 0.5
	// ConfidenceHigh is where a cause is stated plainly. It was
	// calibrated on the labelled scenarios (scorecard.CalibratedHigh):
	// causes scored 0.70 were right 92% of the time, so a strong row
	// whose evidence does not argue against it is stated plainly.
	ConfidenceHigh = 0.70
)

// Priors that are not rows of the table.
const (
	// SelfPrior is the belief that a failing workload is its own
	// cause. It is under the floor on purpose: "the app is broken"
	// must be earned by evidence that its environment is healthy.
	SelfPrior = 0.4
	// SelfFailingPrior is the belief that an entity Failing on its
	// own (a node that stopped reporting) is its own cause when
	// nothing upstream explains it. It sits above the floor: the
	// failure is real even when its reason is not visible.
	SelfFailingPrior = 0.5
	// HopDecay lowers a row's prior for every link beyond the first,
	// so a near cause beats an equally good far one.
	HopDecay = 0.97
)

// Temporal order: a cause starts before its effects.
const (
	// TemporalBefore rewards a cause that began first. It is small:
	// many things start before a failure without causing it.
	TemporalBefore = 0.1
	// TemporalAfter punishes a cause that began after its effects,
	// which is strong evidence against it.
	TemporalAfter = 0.2
	// TemporalSlack absorbs clock skew between sources and the time
	// a detector needs to notice a condition.
	TemporalSlack = time.Minute
	// TemporalExclusion is how long after an incident was opened a
	// cause may still have begun and take the incident over as its
	// revised cause. A cause that began later did not cause what people
	// were already told about: a node pool that starts failing this
	// morning does not explain a deployment that has been unavailable
	// for a week, and the incident keeps its root. The incident layer
	// applies it (incident.Manager.attach); explain reports when each
	// cause began (Cause.Began).
	TemporalExclusion = 10 * time.Minute
)

// Coverage: the share of a candidate's dependents that fail.
const (
	// CoverageWeight is the most coverage adds or subtracts. All
	// dependents failing adds it; none failing subtracts it.
	CoverageWeight = 0.2
	// CoverageMinDependents is the fewest dependents for a share to
	// mean anything; one of one is not a pattern.
	CoverageMinDependents = 2
)

// Exclusivity: the effect's siblings that do not depend on the
// candidate are healthy.
const (
	// ExclusivityWeight rewards healthy siblings elsewhere: the
	// difference between them and the failures is the candidate.
	ExclusivityWeight = 0.15
	// ExclusivityPenalty punishes failing siblings elsewhere: the
	// problem follows the workload, not the candidate.
	ExclusivityPenalty = 0.2
	// ExclusivitySample bounds the effects and siblings compared.
	ExclusivitySample = 8
)

// Specificity: the error text names the candidate.
const (
	// SpecificityWeight rewards an error that names the candidate:
	// words are the most direct evidence Kubernetes gives.
	SpecificityWeight = 0.15
	// SpecificityMinName is the shortest name worth looking for;
	// shorter names match by accident.
	SpecificityMinName = 3
)

// Recent change and revision.
const (
	// ChangeWeight rewards a change inside the causal window before
	// the failures began: most outages follow a change.
	ChangeWeight = 0.15
	// RevisionWeight rewards failures confined to the newest
	// revision while older revisions stay healthy.
	RevisionWeight = 0.15
	// RevisionPenalty punishes a rollout blamed for failures that
	// every revision shows alike.
	RevisionPenalty = 0.15
)

// Change outcomes, judged by the inventory after a change set's
// effect window.
const (
	// OutcomeRevertedWeight rewards a change whose revert brought its
	// dependents back: the strongest proof of causation a cluster
	// gives without asking anyone.
	OutcomeRevertedWeight = 0.1
	// OutcomeDegradedWeight rewards a change whose dependents failed
	// inside its effect window. It is small: the change scorer
	// already rewards the timing.
	OutcomeDegradedWeight = 0.05
)

// Shared dimension: failures of several workloads meet in one place.
const (
	// SharedWeight rewards a candidate that is the one thing several
	// failing workloads have in common.
	SharedWeight = 0.15
	// SharedPenalty punishes a shared candidate (a node, a registry)
	// blamed for one workload only: a shared cause would hurt more.
	SharedPenalty = 0.2
	// SharedMinWorkloads is the fewest workloads that make a pattern.
	SharedMinWorkloads = 2
)

// Failures read from error text: an endpoint several workloads call,
// or one error message several workloads share.
const (
	// EndpointMinWorkloads is the fewest workloads that must fail on
	// one named endpoint before it is blamed. The error names it, so
	// two apps already make a pattern.
	EndpointMinWorkloads = 2
	// SignatureMinWorkloads is the fewest workloads that must share
	// one error before the error itself becomes the incident. Words
	// are weaker than a name, so it takes one more.
	SignatureMinWorkloads = 3
	// SignatureWindow is how close together the failures that share
	// one error must have begun. One outage breaks its callers within
	// minutes; the same words much later are another event.
	SignatureWindow = 30 * time.Minute
)

// Baseline deviation and data quality.
const (
	// BaselineWeight is the most a baseline adds or subtracts. It is
	// small: a baseline says "unusual", not "cause".
	BaselineWeight = 0.1
	// DataQualityPenalty is taken once for each kind of doubt: an
	// Unknown health, or an absence in a kind that is not synced.
	// Unknown data lowers confidence and never raises it.
	DataQualityPenalty = 0.1
)

// Common factors.
const (
	// SharedFactorWindow is how close together the failures of several
	// workloads must have begun for the node they share to be
	// suspected although it shows nothing wrong.
	SharedFactorWindow = 10 * time.Minute
	// SharedFactorMinWorkloads is how many workloads must fail together
	// before a healthy thing they share is suspected.
	SharedFactorMinWorkloads = 3
	// SharedFactorMaxConfidence caps a shared factor: with no finding
	// and no change of its own it is a suspect, worded "possibly", and
	// any cause with evidence of its own outranks it.
	SharedFactorMaxConfidence = 0.49
	// SharedFactorMinShare is the share of a shared factor's dependents
	// that must fail. Why: a node where most pods are fine is not what
	// breaks the failing ones.
	SharedFactorMinShare = 0.5
)

// Groups of nodes.
const (
	// MinGroupMembersFailing is the fewest broken nodes that make a
	// zone or a node pool a candidate. The row's MinCovered counts
	// failures of any kind, so one failing node and its pods would
	// otherwise blame the zone. Only failing node findings count; see
	// view.broken.
	MinGroupMembersFailing = 2
)

// Node lifecycle.
const (
	// NodeReplacementWindow is how soon after a node is deleted a pod
	// must start waiting to count as one of its replacements. The
	// ReplicaSet controller recreates lost pods within seconds; ten
	// minutes also covers pod garbage collection and a slow controller,
	// while a node removed earlier is unrelated to a new pending pod.
	NodeReplacementWindow = 10 * time.Minute
)

// Workload configuration.
const (
	// OOMSteadyRun is the longest run that still reads as "the limit
	// is too low" when it ends OOM-killed: the application needs more
	// than the limit as soon as it is up. A leak grows for longer
	// before it reaches the same limit.
	OOMSteadyRun = 5 * time.Minute
)

// Limits of the output.
const (
	// MaxAlternatives bounds the alternatives kept per area.
	MaxAlternatives = 5
)
