package scenarios

import "time"

// nodeRecoversWorkloadStaysDown: a node that pages is back after five
// minutes, but a workload that ran on it takes eight more to become
// ready. The engine then explains the failure by the workload alone, so
// the incident is moved to it. It keeps its page tier (a page is never
// un-paged), and its update must say the node recovered, or the thread
// would go on believing the node is down.
func nodeRecoversWorkloadStaysDown() scenario {
	return scenario{
		expect: expectation{
			Name: "node-recovers-workload-stays-down",
			Description: "A node that paged recovers while a workload " +
				"that ran on it stays unready: the update that moves " +
				"the incident to the workload says the node recovered.",
			Root: "deployment/shop/cart", Tier: "page", MaxMessages: 3,
			MaxPages: 1,
			Tail:     duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			w := c.deployment("shop", "cart",
				"registry.example.com/cart:1", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(time.Minute)
			setNodeReady(c, n1, false)
			c.after(40 * time.Second)
			c.update(w.pod(0, "n1", notReady))
			w.setReady(1)
			c.update(w.deployment, w.replicaSet)
			c.after(5 * time.Minute)
			setNodeReady(c, n1, true)
			c.after(8 * time.Minute)
			c.update(w.pod(0, "n1"))
			w.setReady(2)
			c.update(w.deployment, w.replicaSet)
			c.after(10 * time.Minute)
		},
	}
}
