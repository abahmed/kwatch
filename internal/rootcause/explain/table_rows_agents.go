package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkNodeAgent links an effect to a pod of a node-agent DaemonSet
// running on its node: one of kube-system, or one of any namespace that
// runs on every node. Such a pod (the CNI, kube-proxy, a CSI node
// plugin, a log shipper) serves every pod on its node; when it fails,
// the pods beside it lose their network, their storage or their
// readiness without any fault of their own.
const LinkNodeAgent LinkType = "node-agent"

// Event-only modes: a mount or attach the kubelet or the attach-detach
// controller reported failing, named by the event reason.
const (
	modeFailedMount  detection.Mode = "FailedMount"
	modeFailedAttach detection.Mode = "FailedAttachVolume"
)

// agentCause and agentEffect are the sides every agent row shares.
var (
	agentCause = Side{Kind: kube.KindPod, Modes: []detection.Mode{
		detection.ModeCrashLoop, detection.ModeNotReady,
		detection.ModeProbe, detection.ModeRestarting,
		detection.ModeError, detection.ModeExit, detection.ModeOOMKilled,
		detection.ModeCreateError, detection.ModeCreating}}
	agentEffectModes = []detection.Mode{
		detection.ModeNotReady, detection.ModeProbe,
		detection.ModeCrashLoop, detection.ModeCreating,
		detection.ModeCreateError, detection.ModeRestarting,
		detection.ModeError}
)

// agentRows blame a failing node agent for the pods on its node.
var agentRows = []Row{
	{
		Name: "node-agent-failing", Cause: agentCause,
		Link: LinkNodeAgent, Effect: podSide(agentEffectModes...),
		Prior: 0.6,
		// One failing workload beside a failing agent may be a
		// coincidence; two share the agent.
		MinWorkloads: 2,
	},
	{
		// One workload is enough when its own error names what the
		// agent does: the network plugin, a storage driver.
		Name: "node-agent-named-by-error", Cause: agentCause,
		Link: LinkNodeAgent,
		Effect: Side{Kind: kube.KindPod, Signal: SignalNodeAgent,
			Modes: append([]detection.Mode{modeFailedMount,
				modeFailedAttach}, agentEffectModes...)},
		Prior: 0.65,
	},
	{
		// A storage driver the pod's error names is registered by a
		// DaemonSet that has no pod on the pod's node.
		Name: "node-agent-absent",
		Cause: Side{Kind: kube.KindDaemonSet,
			Modes: []detection.Mode{ModeAgentAbsent}},
		Link: LinkNodeAgent,
		Effect: Side{Kind: kube.KindPod, Signal: SignalNodeAgent,
			Modes: append([]detection.Mode{modeFailedMount,
				modeFailedAttach}, agentEffectModes...)},
		Prior: 0.65,
	},
}

// agentHops lead from a failing pod to the node-agent DaemonSet pods on
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
	return append(out, v.absentAgentHops(pod)...)
}

// nodeAgent reports a pod of a DaemonSet other than the failing pod's
// own workload that serves its node: any DaemonSet of kube-system, or a
// DaemonSet of another namespace that runs on every node.
func (v *view) nodeAgent(pod, own inventory.EntityID) bool {
	top := rootcause.TopOwner(v.s.Model, pod)
	if top.Kind != kube.KindDaemonSet || top == own {
		return false
	}
	return pod.Namespace == "kube-system" || v.runsOnEveryNode(top)
}

// runsOnEveryNode reports a DaemonSet with a pod on each of at least
// two nodes of the cluster: one that serves every node, as an agent
// does, and not a workload that happens to be a DaemonSet.
func (v *view) runsOnEveryNode(ds inventory.EntityID) bool {
	nodes := v.s.Model.Entities(kube.KindNode)
	if len(nodes) < 2 {
		return false
	}
	hosting := map[inventory.EntityID]bool{}
	for _, pod := range v.s.Model.Related(ds, inventory.OwnedBy,
		inventory.Incoming) {
		for _, node := range v.s.Model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			hosting[node] = true
		}
	}
	for _, node := range nodes {
		if !hosting[node] {
			return false
		}
	}
	return true
}
