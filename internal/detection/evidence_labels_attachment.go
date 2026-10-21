package detection

// Evidence labels of a volume attachment whose deletion is stuck, and
// of any object whose deletion a finalizer holds.
const (
	// EvidenceFinalizers lists the finalizers that hold a deletion,
	// joined with ", ".
	EvidenceFinalizers = "finalizers"
	// EvidenceLastWarning is the message of the latest Warning event on
	// an object whose deletion is stuck, as its controller wrote it. A
	// controller outside kwatch's view says there why it cannot finish.
	EvidenceLastWarning = "latest warning event"
	// EvidenceAttachVolume is the volume an attachment attaches.
	EvidenceAttachVolume = "attached volume"
	// EvidenceAttachNode is the node it attaches the volume to.
	EvidenceAttachNode = "attached to node"
	// EvidenceAttachDriver is the CSI driver that attaches the volume.
	EvidenceAttachDriver = "attach driver"
	// EvidenceAttachGone says what of the attachment no longer exists:
	// "volume", "node" or "volume and node".
	EvidenceAttachGone = "no longer exists"
)
