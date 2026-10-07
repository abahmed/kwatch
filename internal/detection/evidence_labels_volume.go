package detection

// Evidence labels of a claim that is nearly full.
const (
	// EvidenceVolumeSize is what the claim holds and how much of it is
	// used: "18Gi of 20Gi".
	EvidenceVolumeSize = "size"
	// EvidenceVolumeInodes is the share of a claim's inodes in use:
	// "99%".
	EvidenceVolumeInodes = "inodes used"
	// EvidenceVolumeUsedBy names the workloads that mount the claim:
	// "postgres, worker".
	EvidenceVolumeUsedBy = "used by"
)
