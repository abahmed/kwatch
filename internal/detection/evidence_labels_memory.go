package detection

// Evidence labels of the context of an out-of-memory kill or a memory
// eviction: the history kwatch kept, which limit was reached, the node's
// memory, and the bounds a new limit must fit. All of it is observed.
const (
	// EvidenceMemoryRequest is the killed container's memory request.
	EvidenceMemoryRequest = "memory request"
	// EvidenceOOMKind says which limit was reached: "its own memory
	// limit" or "the node's memory".
	EvidenceOOMKind = "killed by limit of"
	// EvidenceMemoryHistory is "none before the kill" when kwatch has no
	// memory reading of the container at all.
	EvidenceMemoryHistory = "memory history"
	// EvidenceMemoryWindow is how much usage history there is, at most
	// 24 hours: "5h".
	EvidenceMemoryWindow = "memory history window"
	// EvidenceMemoryFlat is a run that stayed near one level until it
	// was killed: "near 500Mi for 5h".
	EvidenceMemoryFlat = "memory flat"
	// EvidenceMemoryNearLimit is how many hours of the window came
	// within 5% of the peak, when the peak is at the limit: "6".
	EvidenceMemoryNearLimit = "hours near limit"
	// EvidenceMemoryRuns is where the last runs ended: "4 runs, each
	// near 510Mi" or "3 runs, between 200Mi and 512Mi".
	EvidenceMemoryRuns = "memory runs ended"
	// EvidenceNodeMemory is the node's memory at the kill: "requested
	// 11.2Gi of 12Gi allocatable; 11.8Gi in use".
	EvidenceNodeMemory = "node memory"
	// EvidenceOverRequest is the victim's use against its request:
	// "used 900Mi, over its 256Mi request".
	EvidenceOverRequest = "use against request"
)

// Evidence labels of the bounds a raised memory limit must fit, read by
// the fix step so it never offers a value the cluster refuses.
const (
	// EvidenceMemoryMaxAllowed is the smallest container memory maximum
	// of the namespace's LimitRanges.
	EvidenceMemoryMaxAllowed = "memory limit max allowed"
	// EvidenceMemoryQuotaLeft is the least limits.memory left under the
	// namespace's ResourceQuotas.
	EvidenceMemoryQuotaLeft = "memory quota left"
	// EvidenceMemoryNodeMax is the largest allocatable memory of a node.
	EvidenceMemoryNodeMax = "largest node memory"
	// EvidenceReplicas is how many replicas the workload runs.
	EvidenceReplicas = "replicas"
	// EvidenceBoundsUnread lists the kinds kwatch cannot read, so a new
	// limit was not checked against them: "LimitRange, ResourceQuota".
	EvidenceBoundsUnread = "bounds not read"
	// EvidenceCPUPeak is the highest CPU use of the last 24 hours, and
	// EvidenceCPUAtLimit the CPU used against the limit when the
	// container runs at its limit without heavy throttling: "249m".
	EvidenceCPUPeak    = "cpu peak 24h"
	EvidenceCPUAtLimit = "cpu used at limit"
)
