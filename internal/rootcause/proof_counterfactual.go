package rootcause

// Proof codes of the counterfactual checks: comparisons with healthy
// objects elsewhere that rule a cause in or out (explain's
// counterfactual scorers).
const (
	// ProofImageElsewhere: the failing image runs fine in another
	// workload, so it is not the image itself (counterfactual).
	ProofImageElsewhere ProofCode = "image-elsewhere"
	// ProofConfigElsewhere: a ConfigMap or Secret is used by healthy
	// workloads too (counterfactual).
	ProofConfigElsewhere ProofCode = "config-elsewhere"
	// ProofReplicasNodeLocal: the failing replicas share one node while
	// the others run healthy elsewhere (counterfactual).
	ProofReplicasNodeLocal ProofCode = "replicas-node-local"
	// ProofReplicasEverywhere: every replica fails, on several nodes
	// (counterfactual).
	ProofReplicasEverywhere ProofCode = "replicas-everywhere"
	// ProofNodePeersHealthy: Count other pods on the failing pods' node
	// are healthy, so the node is not the cause (counterfactual).
	ProofNodePeersHealthy ProofCode = "node-peers-healthy"
	// ProofPreviousHealthy: the previous revision ran healthy for Count
	// minutes before the change (counterfactual).
	ProofPreviousHealthy ProofCode = "previous-healthy"
)
