package explain

import "github.com/abahmed/kwatch/internal/inventory/kube"

// sharedRows suspect what several workloads failing at the same time
// have in common when nothing else explains them. The cause shows no
// finding and no change (ModeSharedFactor), so the priors are low and
// each row needs SharedFactorMinWorkloads workloads whose failures
// began within SharedFactorWindow; applySharedFactorWindow enforces
// the window. A node is the most likely common factor: a kernel or
// runtime fault shows in the pods before it shows in the node.
var sharedRows = []Row{
	{
		Name:   "shared-node",
		Cause:  Side{Kind: kube.KindNode, Modes: sharedFactorModes},
		Link:   LinkRunsOn,
		Effect: anything, Prior: 0.4,
		MinWorkloads: SharedFactorMinWorkloads,
	},
	{
		// The same image failing in several workloads is usually a
		// bad build they all picked up.
		Name:   "shared-image",
		Cause:  Side{Kind: kube.KindImage, Modes: sharedFactorModes},
		Link:   LinkPulls,
		Effect: anything, Prior: 0.45,
		MinWorkloads: SharedFactorMinWorkloads,
	},
	{
		Name:   "shared-configmap",
		Cause:  Side{Kind: kube.KindConfigMap, Modes: sharedFactorModes},
		Link:   LinkUses,
		Effect: anything, Prior: 0.4,
		MinWorkloads: SharedFactorMinWorkloads,
	},
	{
		Name:   "shared-secret",
		Cause:  Side{Kind: kube.KindSecret, Modes: sharedFactorModes},
		Link:   LinkUses,
		Effect: anything, Prior: 0.4,
		MinWorkloads: SharedFactorMinWorkloads,
	},
	{
		Name:   "shared-account",
		Cause:  Side{Kind: kube.KindAccount, Modes: sharedFactorModes},
		Link:   LinkUses,
		Effect: anything, Prior: 0.35,
		MinWorkloads: SharedFactorMinWorkloads,
	},
}
