package explain

import "time"

// Weights of the counterfactual checks: what a comparison with healthy
// objects elsewhere adds to or takes from a cause. Like every weight
// they are added to a row's prior (see weights.go). They are small on
// purpose: coverage and exclusivity already count healthy dependents
// and siblings, so a comparison only breaks ties between causes that
// the other evidence leaves close, and it never decides alone.
const (
	// ImageElsewhereWeight is both the reward and the penalty of the
	// same-image check. The same image running healthy in another
	// workload lowers a cause that blames the image (a bad build, a
	// moved tag) and raises a cause that blames what surrounds it
	// (config, environment, a dependency).
	ImageElsewhereWeight = 0.1
	// ConfigElsewhereWeight is taken from a ConfigMap or Secret that
	// healthy workloads use as well: the same bytes work elsewhere.
	ConfigElsewhereWeight = 0.1
	// NodeLocalPenalty is taken from a workload-level cause when two or
	// more failing replicas share one node and the others are healthy
	// on other nodes: the node is the difference, not the workload.
	NodeLocalPenalty = 0.1
	// NodeLocalMinFailing is the fewest failing replicas on one node
	// that make a node-local pattern; one pod on a node is a pod.
	NodeLocalMinFailing = 2
	// EverywhereWeight rewards a workload-level cause when every
	// replica fails on at least two nodes: nothing about a node
	// explains that. It is half the other weights because coverage
	// already rewards a cause whose dependents all fail.
	EverywhereWeight = 0.05
	// NodePeersWeight rewards a workload-level cause when its node
	// runs several other healthy pods of other workloads: the node is
	// fine, so what differs is the workload. The shared-node rules keep
	// their own safeguards; this never raises a node.
	NodePeersWeight = 0.05
	// NodePeersMin is the fewest healthy pods of other workloads that
	// clear a node. Two could be a coincidence.
	NodePeersMin = 3
	// PreviousHealthyWeight rewards a rollout whose previous revision
	// ran healthy before the change.
	PreviousHealthyWeight = 0.05
	// PreviousHealthyMin is how long the previous revision must have
	// been ready before the change: long enough to have shown crashes
	// and failing probes.
	PreviousHealthyMin = 10 * time.Minute
)
