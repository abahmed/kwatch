package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// ModeAgentAbsent is a node agent DaemonSet with no pod on a node whose
// pods need it: the storage driver it registers is missing there.
const ModeAgentAbsent detection.Mode = "AgentAbsent"

// driverErrors find the storage driver a kubelet error names, in
// lower-case text: the one the node's kubelet cannot find registered.
var driverErrors = []*regexp.Regexp{
	regexp.MustCompile(`driver name (\S+) not found in the list of ` +
		`registered csi drivers`),
	regexp.MustCompile(`csinode \S+ does not contain driver (\S+?)[.,;]?` +
		`(?:\s|$)`),
}

// driverIn is the storage driver the effect's error text names.
func (v *view) driverIn(effect inventory.EntityID) string {
	return firstNameIn(strings.ToLower(v.errorText(effect)),
		driverErrors...)
}

// applyDriverGate drops the agent coverage of a failure whose error
// names a storage driver the agent does not register. A failing proxy
// or log shipper beside a pod that cannot mount a volume is not why the
// driver is missing; the agent's own arguments name what it registers.
func (v *view) applyDriverGate(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		var wrong []inventory.EntityID
		for _, effect := range coveredByRow(c, agentNamedRow) {
			if name := v.driverIn(effect); name != "" &&
				!v.registersDriver(c.id, name) {
				wrong = append(wrong, effect)
			}
		}
		v.dropSharedCoverage(cs, c, wrong, "the error names a "+
			"storage driver this agent does not register")
	}
}

// absentAgentHops lead from a pod whose error names a storage driver to
// the DaemonSets that register it with no pod on the pod's node.
func (v *view) absentAgentHops(pod inventory.EntityID) []hop {
	var out []hop
	for _, ds := range v.absentAgents(pod) {
		out = append(out, hop{link: LinkNodeAgent, to: ds})
	}
	return out
}

// absentAgents lists the DaemonSets that run the storage driver the
// pod's error names on other nodes but have no pod on the pod's node.
func (v *view) absentAgents(pod inventory.EntityID) []inventory.EntityID {
	driver := v.driverIn(pod)
	node, ok := v.nodeOf(pod)
	if driver == "" || !ok {
		return nil
	}
	var out []inventory.EntityID
	for _, ds := range v.s.Model.Entities(kube.KindDaemonSet) {
		pods := rootcause.OwnedPods(v.s.Model, ds)
		if v.anyRegisters(pods, driver) && !v.anyOnNode(pods, node) {
			v.gate(pod, ds)
			out = append(out, ds)
		}
	}
	return out
}

func (v *view) anyRegisters(
	pods []inventory.EntityID, driver string,
) bool {
	return anyMember(pods, func(p inventory.EntityID) bool {
		return v.registersDriver(p, driver)
	})
}

func (v *view) anyOnNode(
	pods []inventory.EntityID, node inventory.EntityID,
) bool {
	return anyMember(pods, func(p inventory.EntityID) bool {
		n, ok := v.nodeOf(p)
		return ok && n == node
	})
}

// agentModes are the pseudo modes of an agent as seen from a pod on its
// node: its containers' own findings (a crash-looping agent needs no
// wait for its pod to be unready), and the absence of a DaemonSet with
// no pod on the node.
func (v *view) agentModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkNodeAgent {
		return nil
	}
	switch id.Kind {
	case kube.KindPod:
		return v.containerModes(id)
	case kube.KindDaemonSet:
		pod, ok := v.unitOf(effect)
		if ok && containsID(v.absentAgents(pod), id) {
			return pseudoFailing(ModeAgentAbsent)
		}
	}
	return nil
}

// containerModes are the unhealthy findings of a pod's containers.
func (v *view) containerModes(pod inventory.EntityID) []modeHealth {
	var out []modeHealth
	for _, container := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		out = append(out, findingModes(v.s.Findings[container])...)
	}
	return out
}
