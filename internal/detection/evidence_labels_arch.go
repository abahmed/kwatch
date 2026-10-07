package detection

// Evidence labels of a container that cannot run its image because the
// node has another CPU architecture.
const (
	// EvidenceArchNode is the node and its architecture, written as
	// "arm64 node ip-10-0-1-9". The error line is in EvidenceError.
	EvidenceArchNode = "wrong architecture"
	// EvidenceArchHealthy is how many pods of the same workload run
	// fine on nodes of another architecture, written as "2 pods on
	// amd64 nodes". Absent when there are none.
	EvidenceArchHealthy = "healthy on other architecture"
)
