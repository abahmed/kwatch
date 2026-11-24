package incident

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// waitRows are the causes that state a wait on an operation that is
// under way, not a fault: an object waits for a restore to finish.
var waitRows = map[string]bool{
	explain.RuleClaimRestoresFromSnapshot: true,
}

// unaffected reports an incident that no workload feels: no pod fails,
// nothing is served worse and nothing is lost, and it is either made
// only of the conditions of nodes (pressure that evicts nothing) or
// explained by a wait on an operation under way. It waits for the
// digest; a failure that joins it, or traffic it breaks, makes it speak
// (the tier follows the members until it is announced).
func unaffected(p *Incident) bool {
	if p.trafficLost || p.servingDown || p.hostedFailing ||
		len(p.Members) == 0 || p.Grouped() {
		return false
	}
	if p.Cause != nil && waitRows[p.Cause.Rule] {
		return !impactsWorkload(p)
	}
	if len(p.Impact) > 0 {
		return false
	}
	found := false
	for key, s := range p.Members {
		switch {
		case s.Advisory:
		case key.Entity.Kind != kube.KindNode || nodeLostFinding(s):
			return false
		default:
			found = true
		}
	}
	return found
}

// hostedFailing reports a pod running on node that is not ready: what
// the node's condition may be doing to the workloads placed on it. A
// pod that finished (a completed Job run, an evicted pod) is never
// ready again and says nothing about the node now.
func hostedFailing(model inventory.Reader, node inventory.EntityID) bool {
	if node.Kind != kube.KindNode {
		return false
	}
	for _, pod := range model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		e, ok := model.Entity(pod)
		if !ok || kube.PodFinished(e) {
			continue
		}
		if ready, known := e.Attribute(kube.AttrReady); known {
			if up, _ := ready.Value.AsBool(); !up {
				return true
			}
		}
	}
	return false
}

// impactsWorkload reports an incident whose impact or failures hold a
// workload or one of its pods.
func impactsWorkload(p *Incident) bool {
	ids := append([]inventory.EntityID(nil), p.Impact...)
	for key, s := range p.Members {
		if !s.Advisory {
			ids = append(ids, key.Entity)
		}
	}
	for _, id := range ids {
		if IsWorkload(id.Kind) || id.Kind == kube.KindPod ||
			id.Kind == kube.KindContainer {
			return true
		}
	}
	return false
}
