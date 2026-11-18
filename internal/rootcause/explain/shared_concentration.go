package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// notConcentrated says why the failures on a suspect node are not
// concentrated there, or "" when they are. Co-location is not
// evidence: on a cluster whose pods mostly share one node, any outage
// of a database or a cache fails pods there. The node is suspected
// only when its failing share is SharedFactorConcentration times the
// failing share of the pods on every other node, and at least
// SharedFactorMinElsewhere pods run elsewhere to compare with. Pods
// that are healthy on the node count against it through its own share.
func (v *view) notConcentrated(node inventory.EntityID) string {
	if node.Kind != kube.KindNode {
		return ""
	}
	onFail, onTotal := shareOf(v.dependents(node), v.unitFailing)
	offFail, offTotal := v.failingElsewhere(node)
	if offTotal < SharedFactorMinElsewhere {
		return fmt.Sprintf("only %d pods run on other nodes to compare "+
			"with, so sharing this node proves nothing", offTotal)
	}
	if onTotal == 0 {
		return ""
	}
	on := float64(onFail) / float64(onTotal)
	off := float64(offFail) / float64(offTotal)
	if on < SharedFactorConcentration*off {
		return fmt.Sprintf("pods fail on other nodes too (%d of %d), "+
			"so the failures are not concentrated on it", offFail,
			offTotal)
	}
	return ""
}

// failingElsewhere counts the failing pods, and all pods, that run on
// nodes other than node. The cluster's totals are counted once per
// solve (podTally).
func (v *view) failingElsewhere(node inventory.EntityID) (int, int) {
	if v.podTally == nil {
		v.podTally = &[2]int{}
		for _, n := range v.s.Model.Entities(kube.KindNode) {
			f, t := shareOf(v.dependents(n), v.unitFailing)
			v.podTally[0] += f
			v.podTally[1] += t
		}
	}
	onFail, onTotal := shareOf(v.dependents(node), v.unitFailing)
	return v.podTally[0] - onFail, v.podTally[1] - onTotal
}

// errorSharedOffNode reports a pod whose own error line (signatureOf)
// is also the error of a failing pod on another node that is not
// failing itself: a database or cache refusing everyone shows on
// every node, and the node only hosts the pods that log it.
func (v *view) errorSharedOffNode(node, pod inventory.EntityID) bool {
	name, ok := v.signatureOf(pod)
	if !ok {
		return false
	}
	for at := range v.healthyErrorNodes()[name] {
		if at != node {
			return true
		}
	}
	return false
}

// healthyErrorNodes indexes, once per solve, the error signatures of
// the failing pods on nodes that are not failing.
func (v *view) healthyErrorNodes() map[string]map[inventory.EntityID]bool {
	if v.errorNodes != nil {
		return v.errorNodes
	}
	v.errorNodes = map[string]map[inventory.EntityID]bool{}
	for _, node := range v.s.Model.Entities(kube.KindNode) {
		if v.failing(node) {
			continue
		}
		for _, pod := range v.dependents(node) {
			name, ok := v.signatureOf(pod)
			if !ok || !v.unitFailing(pod) {
				continue
			}
			if v.errorNodes[name] == nil {
				v.errorNodes[name] = map[inventory.EntityID]bool{}
			}
			v.errorNodes[name][node] = true
		}
	}
	return v.errorNodes
}
