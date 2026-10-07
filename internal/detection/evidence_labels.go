package detection

// Evidence labels that a message writer reads by name. A detector that
// writes one and a writer that reads it share the constant.
const (
	// EvidenceMemoryLimit is the memory limit of a container killed for
	// exceeding it, such as "512Mi".
	EvidenceMemoryLimit = "memory limit"
	// EvidenceMemoryPeak is the highest memory use of the last 24 hours.
	EvidenceMemoryPeak = "memory peak 24h"
	// EvidenceMemoryRise is a steady climb up to the kill, such as
	// "200Mi to 512Mi over 3h".
	EvidenceMemoryRise = "memory rise"
	// EvidenceKilledAfter is how long a run lasted when it was killed
	// soon after starting, written in words: "30 seconds".
	EvidenceKilledAfter = "killed after"
)

// EvidenceError is the error line a container logged or reported. The
// tracker watches it: the line can arrive after the finding was raised,
// and the incident must then be grouped again by what it says.
const EvidenceError = "error"

// Evidence labels of a Job that runs far longer than usual.
const (
	// EvidenceRunningFor is how long the Job has run: "2h10m".
	EvidenceRunningFor = "running for"
	// EvidenceRecentRuns is how long the CronJob's recent successful
	// runs took: "8–12m".
	EvidenceRecentRuns = "recent runs"
)

// Evidence labels of a workload scaled to zero while still routed to.
const (
	// EvidenceStillRouted is one way traffic still reaches the
	// workload, written as a clause: "Ingress shop/web still routes
	// traffic to it via Service api".
	EvidenceStillRouted = "still routed"
	// EvidenceScaledBy is the field manager that set the replicas to
	// zero, as the API server recorded it.
	EvidenceScaledBy = "scaled by"
	// EvidenceScaledAt is when it did, in RFC 3339.
	EvidenceScaledAt = "scaled at"
)

// EvidenceNeverHealthy is written only for a workload whose first
// rollout has not become healthy yet. Its value is when the workload
// was created, in RFC 3339.
const EvidenceNeverHealthy = "never healthy since"

// EvidenceEditPrefix starts the label of one edit a new revision made to
// its pod template: "edit containers[api].resources.limits.memory". The
// value is "before → after", an empty side written as "unset". At most
// MaxEditEvidence are listed, likeliest culprit first.
const (
	EvidenceEditPrefix = "edit "
	EvidenceEditArrow  = " → "
	MaxEditEvidence    = 3
)

// Evidence labels of a probe that fails while its container is starved
// of CPU.
const (
	// EvidenceCPUThrottled is the share of CPU periods the container was
	// throttled in, such as "72%".
	EvidenceCPUThrottled = "cpu throttled"
	// EvidenceCPULimit is the container's CPU limit, such as "200m".
	EvidenceCPULimit = "cpu limit"
)

// Evidence labels of a container killed by the node running out of
// memory rather than by its own limit.
const (
	// EvidenceKilledByNode is the name of the node that ran out of
	// memory.
	EvidenceKilledByNode = "killed by node"
	// EvidenceMemoryUsed is what the container used before the kill,
	// such as "180Mi".
	EvidenceMemoryUsed = "memory used"
	// EvidenceNodeMemoryUser is one of the biggest memory users on that
	// node, such as "batch/importer 3.1Gi (no limit)". It repeats, the
	// biggest first.
	EvidenceNodeMemoryUser = "node memory user"
)

// Evidence labels of a container that liveness kills while it is still
// starting.
const (
	// EvidenceLivenessGives is the time liveness allows before the kill,
	// with its parts: "40s (10s delay + 3 × 10s)".
	EvidenceLivenessGives = "liveness gives"
	// EvidenceUsualStart is how long the workload's containers usually
	// take from starting to ready, such as "75s".
	EvidenceUsualStart = "usually ready after"
	// EvidenceStartSamples is how many past starts that figure covers,
	// at most the last five.
	EvidenceStartSamples = "starts seen"
	// EvidenceKilledBeforeReady is "true" when the last run ended
	// within the liveness budget and the container declares a readiness
	// probe: it never reached Ready before the kill. It is written only
	// when no history of starts says how long a start takes.
	EvidenceKilledBeforeReady = "killed before ready"
	// EvidenceLivenessSameCheck is "true" when the liveness probe runs
	// the same check as the readiness probe.
	EvidenceLivenessSameCheck = "liveness same check"
)
