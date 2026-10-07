package detection

// Evidence labels of a pod the scheduler preempted.
const (
	// EvidencePreemptor is the pod that took the victim's place, as
	// "namespace/name", read from the scheduler's own message.
	EvidencePreemptor = "preempted by"
	// EvidencePreemptorOwner is the workload of that pod, as
	// "namespace/name", when the model knows it.
	EvidencePreemptorOwner = "preemptor workload"
	// EvidencePriorities compares the two priorities, as "1000 > 0".
	// Absent when either is unknown.
	EvidencePriorities = "priorities"
	// EvidencePreemptedAt is when the scheduler preempted the pod, in
	// RFC 3339.
	EvidencePreemptedAt = "preempted at"
)
