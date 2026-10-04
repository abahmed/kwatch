package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkNodeAgent links an effect to a pod of a kube-system DaemonSet
// running on its node. Such a pod (the CNI, kube-proxy, a CSI node
// plugin, a log shipper) serves every pod on its node; when it fails,
// the pods beside it lose their network, their storage or their
// readiness without any fault of their own.
const LinkNodeAgent LinkType = "node-agent"

// agentRows blame a failing node agent for the pods on its node.
var agentRows = []Row{
	{
		Name: "node-agent-failing",
		Cause: Side{Kind: kube.KindPod, Modes: []detection.Mode{
			detection.ModeCrashLoop, detection.ModeNotReady,
			detection.ModeProbe, detection.ModeRestarting,
			detection.ModeError, detection.ModeExit, detection.ModeOOMKilled,
			detection.ModeCreateError, detection.ModeCreating}},
		Link: LinkNodeAgent,
		Effect: podSide(
			detection.ModeNotReady, detection.ModeProbe,
			detection.ModeCrashLoop, detection.ModeCreating,
			detection.ModeCreateError, detection.ModeRestarting,
			detection.ModeError),
		Prior: 0.6,
		// One failing workload beside a failing agent may be a
		// coincidence; two share the agent.
		MinWorkloads: 2,
	},
}

// agentHops lead from a failing pod to the kube-system DaemonSet pods on
// its node that are failing too. The hop is added only for a failing
// pod, so healthy pods never fan out to their node's agents.
func (v *view) agentHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) {
		return nil
	}
	own := rootcause.TopOwner(v.s.Model, pod)
	var out []hop
	for _, node := range v.s.Model.Related(pod, inventory.RunsOn,
		inventory.Outgoing) {
		for _, other := range v.s.Model.Related(node, inventory.RunsOn,
			inventory.Incoming) {
			if other == pod || !v.nodeAgent(other, own) ||
				!v.unitFailing(other) {
				continue
			}
			out = append(out, hop{link: LinkNodeAgent, to: other})
		}
	}
	return out
}

// nodeAgent reports a pod of a kube-system DaemonSet other than the
// failing pod's own workload.
func (v *view) nodeAgent(pod, own inventory.EntityID) bool {
	if pod.Namespace != "kube-system" {
		return false
	}
	top := rootcause.TopOwner(v.s.Model, pod)
	return top.Kind == kube.KindDaemonSet && top != own
}
