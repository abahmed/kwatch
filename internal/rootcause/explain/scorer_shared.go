package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// sharedKinds are the dimensions many workloads share. A problem of
// one of them shows up in several workloads at once. etcd is left out:
// the API server is its only direct dependent.
var sharedKinds = map[inventory.Kind]bool{
	kube.KindNode: true, kube.KindZone: true, kube.KindNodePool: true,
	kube.KindRegistry: true, kindClusterDNS: true, kindAPIServer: true,
	kube.KindNamespace: true, kindScheduler: true,
	kindControllerManager: true, KindExternalEndpoint: true,
	KindFailureSignature: true,
}

// scoreShared rewards a candidate that is what several failing
// workloads have in common, and punishes a shared candidate (a node,
// a registry) blamed for one workload only: a crash on one app's pods
// is the app's problem even if the pods share a node.
func scoreShared(v *view, c *candidate) outcome {
	effects := c.others()
	if len(effects) == 0 || holdsFinalizers(c) {
		return outcome{}
	}
	workloads := v.workloadsOf(effects)
	switch {
	case workloads >= SharedMinWorkloads:
		return outcome{weight: SharedWeight,
			code: rootcause.ProofWorkloadsMeet, count: workloads,
			text: fmt.Sprintf(
				"failures of %d workloads meet here", workloads)}
	case sharedKinds[c.id.Kind] && !isTopology(c.id):
		return outcome{weight: -SharedPenalty,
			code: rootcause.ProofOneWorkload,
			text: "only one workload fails behind it"}
	}
	return outcome{}
}

// workloadsOf counts the distinct top owners of the effects.
func (v *view) workloadsOf(effects []inventory.EntityID) int {
	owners := map[inventory.EntityID]bool{}
	for _, effect := range effects {
		owners[v.workloadOf(effect)] = true
	}
	return len(owners)
}

// workloadOf is the top owner of an effect, or of its pod.
func (v *view) workloadOf(effect inventory.EntityID) inventory.EntityID {
	unit := effect
	if pod, ok := v.podOf(effect); ok {
		unit = pod
	}
	return rootcause.TopOwner(v.s.Model, unit)
}
