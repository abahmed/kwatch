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
