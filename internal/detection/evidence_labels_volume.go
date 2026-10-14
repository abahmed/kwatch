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

// Evidence labels of a pod that waits for a volume another pod still
// holds (a multi-attach error).
const (
	// EvidenceHeldVolume is the volume the error names.
	EvidenceHeldVolume = "volume"
	// EvidenceHeldPod is the pod that still uses the volume.
	EvidenceHeldPod = "used by pod"
	// EvidenceHeldPodEnding is "terminating" for a pod that was asked to
	// stop and has not finished.
	EvidenceHeldPodEnding = "used by pod is"
)

// Evidence labels of a claim that waits for its provisioner.
const (
	// EvidenceProvisioner is the provisioner of the claim's StorageClass.
	EvidenceProvisioner = "provisioner"
	// EvidenceProvisionerWorkload says what runs the provisioner:
	// "deployment kube-system/ebs-csi-controller, no ready pods" or
	// "none found".
	EvidenceProvisionerWorkload = "provisioner runs as"
)
