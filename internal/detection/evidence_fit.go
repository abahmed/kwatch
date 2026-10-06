package detection

// Evidence labels of the scheduling explainer, which says which nodes
// could take an unschedulable pod. Each "fit" value is one finished
// clause, such as "pool general has no node with 1.5 CPU free (best: n3
// has 1.2 CPU free)".
const (
	// EvidenceFit is one reason no node, or no pool of nodes, fits.
	EvidenceFit = "fit"
	// EvidenceFitWould is a fact about a change that would make the pod
	// fit: "tolerating gpu=true:NoSchedule would fit it on gpu-1".
	EvidenceFitWould = "fit would"
	// EvidenceFitNote says what the check could not look at.
	EvidenceFitNote = "fit note"
)

// Evidence labels of what an autoscaler said about a pending pod.
const (
	// EvidenceAutoscaler is "adding" while the autoscaler is adding a
	// node for the pod, "late" when it said so long ago, or "blocked"
	// when it said it cannot.
	EvidenceAutoscaler = "autoscaler"
	// EvidenceAutoscalerSays is the autoscaler's event message, quoted.
	EvidenceAutoscalerSays = "autoscaler says"
)

// Values of EvidenceAutoscaler.
const (
	AutoscalerAdding  = "adding"
	AutoscalerBlocked = "blocked"
	// AutoscalerLate: it said it was adding a node, long enough ago
	// that the node should have come.
	AutoscalerLate = "late"
)
