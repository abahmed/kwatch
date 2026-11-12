package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// RowNodePullsThrottled names the row that blames a node's kubelet for
// the pulls it held back.
const RowNodePullsThrottled = "node-pulls-throttled"

// ModePullThrottled is a node whose kubelet holds image pulls back
// under its own pull rate limit, seen from the pods that wait on it.
const ModePullThrottled detection.Mode = "PullThrottled"

// nodePullThrottledRow blames the node for pulls its kubelet held back
// ("pull QPS exceeded"): the registry was never asked, so it is not the
// registry refusing. The kubelet retries on its own; many pods starting
// on one node at once is the usual reason.
var nodePullThrottledRow = Row{
	Name: RowNodePullsThrottled,
	Cause: Side{Kind: kube.KindNode,
		Modes: []detection.Mode{ModePullThrottled}},
	Link:   LinkRunsOn,
	Effect: podSide(detection.ModeImagePull),
	Prior:  0.75,
}

// pullThrottledOnNode reports a pod or container whose failed pulls
// the kubelet of its node held back under its pull rate limit.
func (v *view) pullThrottledOnNode(effect inventory.EntityID) bool {
	text, ok := v.pullText(effect)
	return ok && classifyPull(text) == pullNodeThrottled
}
