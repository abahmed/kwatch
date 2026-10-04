package explain

import "github.com/abahmed/kwatch/internal/inventory/kube"

// sharedRows suspect what several workloads failing at the same time
// have in common when nothing else explains them. Only a node is
// suspected: an unchanged image, config, secret or account is weak
// evidence, and bad builds are covered by rollout rows, image digest
// drift and the shared-failure-signature row. The cause shows no
// finding and no change (ModeSharedFactor), so the prior is low and
// the row needs SharedFactorMinWorkloads workloads whose
// failures began within SharedFactorWindow; applySharedFactorWindow
// enforces the window. A node is the most likely common factor: a kernel or
// runtime fault shows in the pods before it shows in the node.
var sharedRows = []Row{
	{
		Name:   "shared-node",
		Cause:  Side{Kind: kube.KindNode, Modes: sharedFactorModes},
		Link:   LinkRunsOn,
		Effect: anything, Prior: 0.4,
		MinWorkloads: SharedFactorMinWorkloads,
	},
}
