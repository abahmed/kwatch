package scenarios

import (
	"fmt"
	"time"
)

// coOnsetScenarios are failures of two workloads that begin seconds
// apart. Two workloads are too few for a shared node on their own; a
// shared moment makes them enough. Two that fail minutes apart, or on
// nodes of their own, stay apart.
func coOnsetScenarios() []scenario {
	return []scenario{
		pairSharesNodeTogether(), pairSharesNodeApart(),
		pairOnSeparateNodesTogether(),
	}
}

// pairCrash crash-loops the first replica of each fleet workload on its
// node, gap apart, five times over.
func pairCrash(
	c *cluster, fleet []*workload, names, nodes []string, gap time.Duration,
) {
	for restarts := int32(1); restarts <= 5; restarts++ {
		for i, w := range fleet {
			c.update(w.pod(0, nodes[i], crashLoop(1, "Error",
				fmt.Sprintf("fatal: %s worker lost its socket", names[i]),
				restarts)))
			w.setReady(1)
			c.update(w.objects())
			c.after(gap)
		}
		c.after(20 * time.Second)
	}
}

// pairSharesNodeTogether: orders and cart each crash-loop their replica
// on node n1, four seconds apart, with different errors, while their
// replicas on n2 stay healthy and n1 reports nothing. Two workloads
// began failing as one moment on the node they share.
func pairSharesNodeTogether() scenario {
	names := []string{"orders", "cart"}
	return scenario{
		expect: expectation{
			Name: "co-onset-pair-shares-node",
			Description: "Two Deployments crash-loop their replicas on " +
				"one healthy node four seconds apart; replicas " +
				"elsewhere are fine.",
			Root: "node//n1", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/cart", "zone//zone-a"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop", names, []string{"n1", "n2"})
			c.after(2 * time.Minute)
			pairCrash(c, fleet, names, []string{"n1", "n1"},
				4*time.Second)
		},
	}
}

// pairSharesNodeApart: the same two Deployments on the same node, but
// the second begins failing three minutes after the first. Nothing
// ties the two moments, so no node is suspected.
func pairSharesNodeApart() scenario {
	names := []string{"orders", "cart"}
	return scenario{
		expect: expectation{
			Name: "adv-co-onset-pair-minutes-apart",
			Description: "Two Deployments crash-loop their replicas on " +
				"one healthy node three minutes apart; the node is " +
				"not suspected.",
			Root:       "deployment/shop/orders",
			OtherRoots: []string{"deployment/shop/cart"},
			Tier:       "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//n1", "zone//zone-a"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop", names, []string{"n1", "n2"})
			c.after(2 * time.Minute)
			pairCrash(c, fleet, names, []string{"n1", "n1"},
				3*time.Minute)
		},
	}
}

// pairOnSeparateNodesTogether: two Deployments crash-loop within four
// seconds of each other, each on a node of its own. They share nothing,
// so each is its own incident however close the clocks are.
func pairOnSeparateNodesTogether() scenario {
	names := []string{"orders", "cart"}
	return scenario{
		expect: expectation{
			Name: "adv-co-onset-separate-nodes",
			Description: "Two Deployments crash-loop four seconds apart, " +
				"each on its own node; independent failures stay apart.",
			Root:       "deployment/shop/orders",
			OtherRoots: []string{"deployment/shop/cart"},
			Tier:       "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//n1", "node//n2", "node//n3",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop", names, []string{"n1", "n2", "n3"})
			c.after(2 * time.Minute)
			pairCrash(c, fleet, names, []string{"n1", "n2"},
				4*time.Second)
		},
	}
}
