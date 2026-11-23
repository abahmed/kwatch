package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

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
		MinWorkloads: CoOnsetMinWorkloads,
	},
	{
		// A zone or pool that holds every failing replica of a workload
		// while the replicas outside it run: the group is what differs.
		// Like a node it shows nothing itself; the clean split is the
		// evidence (applySharedFactorWindow).
		Name:   "shared-zone",
		Cause:  Side{Kind: kube.KindZone, Modes: sharedFactorModes},
		Link:   LinkContains,
		Effect: anything, Prior: 0.4,
	},
	{
		Name:   "shared-nodepool",
		Cause:  Side{Kind: kube.KindNodePool, Modes: sharedFactorModes},
		Link:   LinkContains,
		Effect: anything, Prior: 0.4,
	},
	{
		// Pods are bound to a node whose kubelet has reported nothing
		// on them: the node's kubelet is behind, not the workload.
		Name: RuleNodeKubeletSilent,
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{ModeKubeletSilent}},
		Link: LinkRunsOn,
		Effect: Side{Kind: kube.KindPod,
			Modes: []detection.Mode{detection.ModePending}},
		Prior: 0.9,
	},
	{
		// The kubelet of a node ended the pod when the node shut down:
		// the pod's own condition says so, so one pod is enough.
		Name: RuleNodeEndedPod,
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{ModeEndedPods}},
		Link: LinkRunsOn,
		Effect: Side{Kind: kube.KindPod,
			Modes: []detection.Mode{detection.ModeFailed}},
		Prior: 0.8,
	},
	{
		// The kubelet of a node refuses a pod by its own accounting
		// (OutOfcpu, OutOfmemory): the refusal is about that node, so
		// one workload is enough.
		Name: RuleNodeRefusesPods,
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{ModeRefusesPods}},
		Link: LinkRunsOn,
		Effect: Side{Kind: kube.KindPod,
			Modes: []detection.Mode{detection.ModeAdmissionRejected,
				detection.ModeResizeDeferred}},
		Prior: 0.6,
	},
}
