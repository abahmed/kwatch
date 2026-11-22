package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Row names the agent gates read.
const (
	agentFailingRow = "node-agent-failing"
	agentNamedRow   = "node-agent-named-by-error"
)

// applyAgentGates keeps an agent as a candidate only where it is a
// plausible common cause: see applyAgentGate and applyDriverGate.
func (v *view) applyAgentGates(cs *candidateSet) {
	v.applyAgentGate(cs)
	v.applyDriverGate(cs)
	v.releaseAgentUnits(cs)
}

// releaseAgentUnits takes the failures of an agent that explains pods
// beside it away from the shared-node suspect. The agent fails on its
// node because it is the cause there, not because the node is: leaving
// its pod, containers and DaemonSet to the node would make two roots
// for one fault.
func (v *view) releaseAgentUnits(cs *candidateSet) {
	own := map[inventory.EntityID]bool{}
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		if len(coveredByRow(c, agentFailingRow))+
			len(coveredByRow(c, agentNamedRow)) > 0 {
			v.markAgentUnit(own, c.id)
		}
	}
	if len(own) == 0 {
		return
	}
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		var released []inventory.EntityID
		for _, effect := range sharedFactorEffects(c) {
			if own[effect] {
				released = append(released, effect)
			}
		}
		v.dropSharedCoverage(cs, c, released, "it is the failing "+
			"agent that explains the pods on the node")
	}
}

// markAgentUnit adds an agent pod, its containers and its owners.
func (v *view) markAgentUnit(
	into map[inventory.EntityID]bool, pod inventory.EntityID,
) {
	into[pod] = true
	for _, container := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		into[container] = true
	}
	for _, owner := range rootcause.OwnerChain(v.s.Model, pod) {
		into[owner] = true
	}
}

// applyAgentGate keeps the coverage of the generic agent row only where
// a node agent is plausibly the common cause, as the shared-node row
// does for a healthy node. An agent serves every pod on its node, so it
// is not what breaks a few of them: most of the workload pods on its
// node must fail, they must have begun together, and each failing
// pod's workload must run a healthy replica elsewhere (otherwise the
// workload fails the same way on any node and is the likelier culprit).
func (v *view) applyAgentGate(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		effects := coveredByRow(c, agentFailingRow)
		if len(effects) == 0 {
			continue
		}
		reason := v.agentVeto(c.id, effects)
		if reason == "" {
			var lone []inventory.EntityID
			for _, effect := range effects {
				if !v.healthyReplicaElsewhere(effect, c.id) {
					lone = append(lone, effect)
				}
			}
			effects, reason = lone, "no replica of the workload runs "+
				"healthy elsewhere, so the workload may be at fault"
		}
		v.dropSharedCoverage(cs, c, effects, reason)
	}
}

// coveredByRow lists the effects a candidate covers through one row.
func coveredByRow(c *candidate, row string) []inventory.EntityID {
	var out []inventory.EntityID
	for _, effect := range c.direct() {
		if c.covers[effect].match.row.Name == row {
			out = append(out, effect)
		}
	}
	return out
}

// agentVeto says why the agent row must be dropped for the whole node,
// or "" when it may stand.
func (v *view) agentVeto(
	agent inventory.EntityID, effects []inventory.EntityID,
) string {
	if !v.beganTogether(effects) {
		return "the failures it would explain did not begin together"
	}
	for _, node := range v.s.Model.Related(agent, inventory.RunsOn,
		inventory.Outgoing) {
		if !v.mostWorkloadPodsFail(node) {
			return "most of the pods on its node are healthy"
		}
	}
	return ""
}

// mostWorkloadPodsFail reports whether at least SharedFactorMinShare of
// the pods of ordinary workloads on the node fail. The node's agents
// are left out: they are what is being judged, and the healthy ones
// beside it say nothing about the pods that depend on it.
func (v *view) mostWorkloadPodsFail(node inventory.EntityID) bool {
	var pods []inventory.EntityID
	for _, pod := range v.s.Model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		if rootcause.TopOwner(v.s.Model, pod).Kind !=
			kube.KindDaemonSet {
			pods = append(pods, pod)
		}
	}
	if len(pods) == 0 {
		return true
	}
	failing, total := shareOf(pods, v.unitFailing)
	return float64(failing)/float64(total) >= SharedFactorMinShare
}

// AgentUnreadyGrace is how long a node agent's pod must have been not
// ready before it counts as down without a finding: the pods it serves
// fail within minutes, before the not-ready finding is raised.
const AgentUnreadyGrace = time.Minute

// agentDown reports a node agent pod that fails by finding or has been
// not ready for AgentUnreadyGrace.
func (v *view) agentDown(pod inventory.EntityID) bool {
	return v.unitFailing(pod) || v.agentUnready(pod)
}

// agentUnready reports a running pod that has been not ready for
// AgentUnreadyGrace while the rest of its DaemonSet is ready: the
// agent's own fault, not the cluster's.
func (v *view) agentUnready(pod inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(pod)
	if !ok {
		return false
	}
	ready, known := e.Attribute(kube.AttrReady)
	if up, _ := ready.Value.AsBool(); !known || up {
		return false
	}
	since, ok := e.Attribute(kube.AttrReadySince)
	if !ok {
		return false
	}
	at := since.Value.AsTime()
	return !at.IsZero() && v.s.Now.Sub(at) >= AgentUnreadyGrace
}
