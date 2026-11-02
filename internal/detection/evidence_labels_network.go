package detection

// EvidenceSandboxEvent is the newest pod sandbox event message behind a
// node's network finding, as the kubelet reported it. It is quoted, never
// reworded.
const EvidenceSandboxEvent = "sandbox event"

// EvidenceStuckPods counts the pods behind a node's network finding that
// have not started since (not ready and not finished): the ones that
// cannot start now. The "affected pods" count also holds the pods that
// failed the same way within the event window and started since.
const EvidenceStuckPods = "pods not started"
