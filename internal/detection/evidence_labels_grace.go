package detection

// Evidence labels of a pod whose containers were killed when their
// termination grace period ran out.
const (
	// EvidenceGracePeriod is the pod's termination grace period, such
	// as "30s".
	EvidenceGracePeriod = "grace period"
	// EvidenceKilledContainers lists the containers that were killed.
	EvidenceKilledContainers = "killed containers"
	// EvidenceDuringRollout is "true" when the pod's workload was in the
	// middle of a rollout when the pod was killed.
	EvidenceDuringRollout = "during rollout"
	// EvidenceStopHook is the kubelet's own message about a preStop hook
	// or a kill that failed, quoted.
	EvidenceStopHook = "stop hook"
)
