package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// groupingScenarios are outages with several members and the way they
// are paged: one page per outage, however many nodes or pods it hits.
func groupingScenarios() []scenario {
	return []scenario{
		nodeBlipThenDown(), nodesLostSameZone(), nodesLostThreeZones(),
		nodePressureEvictedLinger(), nodeLateCauseAfterCrashes(),
	}
}

// groupNamespaces are the namespaces the grouping fleets live in.
var groupNamespaces = []string{"shop", "billing", "search", "media"}

// groupFleet lists healthy two-replica Deployments, perNS in each of
// the grouping namespaces, one replica on each of the nodes.
func groupFleet(c *cluster, perNS int, nodes []string) []*workload {
	var all []*workload
	for _, ns := range groupNamespaces {
		var names []string
		for i := range perNS {
			names = append(names, fmt.Sprintf("%s-app%d", ns, i))
		}
		all = append(all, nodeFleet(c, ns, names, nodes)...)
	}
	return all
}

// nodeRecover is the node posting Ready again, its taints gone.
func nodeRecover(c *cluster, node *corev1.Node) {
	back := last(c, node)
	setNodeCondition(back, corev1.NodeReady, corev1.ConditionTrue,
		"KubeletReady", "kubelet is posting ready status", c.now)
	back.Spec.Taints = nil
	c.update(back)
}

// nodeBlipThenDown: n1 is lost for four minutes and comes back, then
// is lost again ten minutes later and stays down. The first loss is
// paged; the second is the real outage and is paged once more.
func nodeBlipThenDown() scenario {
	return scenario{
		expect: expectation{
			Alone: true,
			Name:  "node-blip-then-down",
			Description: "A node is lost for four minutes, recovers, " +
				"and fails again within two hours for good: the " +
				"flap is paged once, the lasting outage once more.",
			Root: "node//n1", Tier: "page", MaxMessages: 5, MaxPages: 2,
			Tail: duration(60 * time.Minute),
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			groupFleet(c, 2, []string{"n1", "n2"})
			c.after(time.Minute)
			nodeLose(c, n1)
			c.after(4 * time.Minute)
			nodeRecover(c, n1)
			c.after(10 * time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Minute)
		},
	}
}

// nodesLost loses the nodes at the given positions of twelve nodes
// spread over the zones, each running one replica of the three-replica
// Deployments, and lets the unready pods follow.
func nodesLost(zones []string, down []int) func(c *cluster) {
	return func(c *cluster) {
		isDown := map[int]bool{}
		for _, i := range down {
			isDown[i] = true
		}
		var nodes []*corev1.Node
		var names []string
		for i := range 12 {
			name := fmt.Sprintf("n%d", i+1)
			n := c.node(name, zones[i%len(zones)])
			nodes, names = append(nodes, n), append(names, name)
			c.list(n)
		}
		var fleet []*workload
		for i, ns := range groupNamespaces {
			for j := range 3 {
				w := c.deployment(ns, fmt.Sprintf("%s-app%d", ns, j),
					"registry.example.com/x:1", 3)
				c.list(w.objects())
				for r := range 3 {
					c.list(w.pod(r, names[(i*3+j+r)%12]))
				}
				fleet = append(fleet, w)
			}
		}
		c.after(time.Minute)
		for i, n := range nodes {
			if isDown[i] {
				nodeLose(c, n)
			}
		}
		c.after(40 * time.Second)
		for fi, w := range fleet {
			var ready int32 = 3
			for r := range 3 {
				if idx := (fi + r) % 12; isDown[idx] {
					c.update(w.pod(r, names[idx], notReady))
					ready--
				}
			}
			w.setReady(ready)
			c.update(w.deployment, w.replicaSet)
		}
		c.after(15 * time.Minute)
	}
}

// nodesLostSameZone: three of the twelve nodes, all in one zone, are
// lost in the same minute. One outage, so one page.
func nodesLostSameZone() scenario {
	return scenario{
		expect: expectation{
			Alone: true,
			Name:  "nodes-lost-same-zone",
			Description: "Three of twelve nodes in one zone are lost at " +
				"once: one page for the outage, not one per node.",
			Root: "node//n1", Tier: "page", MaxMessages: 6, MaxPages: 1,
			Tail: duration(20 * time.Minute),
		},
		build: nodesLost([]string{"zone-a"}, []int{0, 1, 2}),
	}
}

// nodesLostThreeZones: five nodes in three zones are lost together.
func nodesLostThreeZones() scenario {
	return scenario{
		expect: expectation{
			Alone: true,
			Name:  "nodes-lost-three-zones",
			Description: "Five nodes across three zones are lost in the " +
				"same minute: one page, and no zone reported healthy " +
				"while its nodes are still down.",
			Root: "node//n1", Tier: "page", MaxMessages: 6, MaxPages: 1,
			Tail: duration(20 * time.Minute),
		},
		build: nodesLost([]string{"zone-a", "zone-b", "zone-c"},
			[]int{0, 1, 2, 3, 4}),
	}
}
