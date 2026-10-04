package scenarios

import (
	"fmt"
	"time"
)

// commonFactorScenarios are several workloads failing at the same time
// with nothing wrong upstream: what they have in common is suspected,
// and workloads that share nothing are not grouped.
func commonFactorScenarios() []scenario {
	return []scenario{sharedNodeSuspected(), separateNodesNotSuspected()}
}

// sharedNodeSuspected: three Deployments each crash-loop their replica
// on node n1 within a minute, with three different errors, while their
// replicas on n2 stay healthy and n1 reports nothing wrong. The node is
// what they share and is suspected, with low confidence.
func sharedNodeSuspected() scenario {
	names := []string{"orders", "payments", "cart"}
	return scenario{
		expect: expectation{
			Name: "shared-node-suspected",
			Description: "Three Deployments crash-loop their replicas on " +
				"one healthy node within a minute, each with its own " +
				"error; replicas elsewhere are fine.",
			Root: "node//n1", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/payments", "deployment/shop/cart",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop", names, []string{"n1", "n2"})
			c.after(2 * time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for i, w := range fleet {
					c.update(w.pod(0, "n1", crashLoop(1, "Error",
						fmt.Sprintf("fatal: %s worker lost its socket",
							names[i]), restarts)))
					w.setReady(1)
					c.update(w.objects())
					c.after(20 * time.Second)
				}
			}
		},
	}
}

// separateNodesNotSuspected: the same three Deployments crash-loop at
// the same time, but each on a different node. They share nothing, so
// each is its own incident and no node is blamed.
func separateNodesNotSuspected() scenario {
	names := []string{"orders", "payments", "cart"}
	return scenario{
		expect: expectation{
			Name: "separate-nodes-not-suspected",
			Description: "Three Deployments crash-loop at the same time, " +
				"each on a different node; nothing is shared.",
			Root: "deployment/shop/orders",
			OtherRoots: []string{"deployment/shop/payments",
				"deployment/shop/cart"},
			Tier: "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//n1", "node//n2", "node//n3",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3"}
			for _, name := range nodes {
				c.list(c.node(name, "zone-a"))
			}
			fleet := nodeFleet(c, "shop", names, []string{"n1", "n2", "n3"})
			c.after(2 * time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for i, w := range fleet {
					c.update(w.pod(i, nodes[i], crashLoop(1, "Error",
						fmt.Sprintf("fatal: %s worker lost its socket",
							names[i]), restarts)))
					w.setReady(2)
					c.update(w.objects())
					c.after(20 * time.Second)
				}
			}
		},
	}
}
