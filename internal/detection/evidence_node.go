package detection

// Evidence labels of a NotReady node that say why, beyond the Ready
// condition's own message.
const (
	// EvidenceReadyStatus is the Ready condition's status when it is not
	// True: "False", or "Unknown" when the kubelet stopped reporting.
	EvidenceReadyStatus = "ready status"
	// EvidenceNodeCondition is another node condition that is True,
	// worded as the condition's name and quoted message.
	EvidenceNodeCondition = "node condition"
	// EvidenceNodeEvent is a recent Warning event of the node, worded
	// as the event's reason and quoted message.
	EvidenceNodeEvent = "node event"
)
