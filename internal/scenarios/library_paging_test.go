package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// pagingScenarios are outages that must page once per team, or notify
// once per namespace, however many incidents the engine opens for them:
// the paging rules of internal/incident and the collecting steps of
// internal/pipeline/announce, judged end to end.
func pagingScenarios() []scenario {
	return []scenario{
		namespaceOutageWithServices(), namespaceOutagePagesOnce(),
		twoTeamsTwoPages(), zoneNodesLostStaggered(),
		nodeRecoversWorkloadStaysDown(),
	}
}

// pagingFleet lists n single-replica Deployments in shop, each behind a
// Service of type st and on a node of its own, so the failures share no
// node. A Service makes a workload explain itself: its cause is the
// workload's own failure, not nothing.
func pagingFleet(
	c *cluster, n int, st corev1.ServiceType,
) ([]*workload, []string) {
	var fleet []*workload
	var nodes []string
	for i := range n {
		node := fmt.Sprintf("n%d", i)
		c.list(c.node(node, "zone-a"))
		name := fmt.Sprintf("app%d", i)
		w := c.deployment("shop", name,
			fmt.Sprintf("registry.example.com/%s:1.0", name), 1)
		c.list(w.deployment, w.replicaSet)
		p := w.pod(0, node)
		c.list(p)
		svc := clusterService(c, "shop", name, 8080)
		svc.Spec.Type = st
		c.list(svc, trafficSlice(c, name, p))
		fleet, nodes = append(fleet, w), append(nodes, node)
	}
	return fleet, nodes
}

// crashServiced crash-loops the first count apps of the fleet, each with
// its own error, taken from outageDependencies from index first on, and
// leaves their Services without a ready endpoint.
func crashServiced(
	c *cluster, fleet []*workload, nodes []string, count, first int,
) {
	c.after(2 * time.Minute)
	for restarts := int32(1); restarts <= 5; restarts++ {
		for i := range count {
			outageCrash(c, fleet[i], nodes[i], first+i, restarts)
			c.update(trafficUnreadySlice(c, fmt.Sprintf("app%d", i),
				fleet[i].pod(0, nodes[i])))
			c.after(time.Second)
		}
		c.after(20 * time.Second)
	}
}

// outageRoots lists the roots of the first count apps of shop's fleet
// after app0, as expectations name them.
func outageRoots(count int) []string {
	var roots []string
	for i := 1; i < count; i++ {
		roots = append(roots, fmt.Sprintf("deployment/shop/app%d", i))
	}
	return roots
}
