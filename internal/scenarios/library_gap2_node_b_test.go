package scenarios

import "time"

// gap2ConntrackFull: the connection tracking table of n2 is full; the
// kernel drops new connections, so the pods there time out dialling the
// database Service while their replicas on n1 work.
func gap2ConntrackFull() scenario {
	return scenario{
		expect: g2Exp("node-conntrack-table-full",
			"A node's connection tracking table is full; pods of three "+
				"workloads on it time out on new connections while "+
				"their replicas on another node work.",
			"node//n2", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart",
			"cluster-dns//cluster-dns").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, []string{"n1", "n2"})
			c.after(2 * time.Minute)
			n2 := last(c, c.node("n2", "zone-a"))
			g2Event(c, n2, "Node", "NetfilterConntrackFull", "nf_conntrack: "+
				"table full, dropping packet", "kernel-monitor", 40)
			gap2NodeCrash(c, fleet, "n2", 1, "dial tcp 10.96.14.7:5432: "+
				"i/o timeout", 6)
		},
	}
}

// gap2MaxMapCount: three replicas of a search engine run on three nodes;
// the one on n3, which still has the default vm.max_map_count, exits at
// its bootstrap check while the two on tuned nodes run.
func gap2MaxMapCount() scenario {
	return scenario{
		expect: g2Exp("node-max-map-count-too-low",
			"One replica of a search engine exits at its bootstrap "+
				"check because its node's vm.max_map_count is too low; "+
				"the replicas on the other nodes run.",
			"node//n3", "notify", "deployment/shop/search").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "search", "registry.example.com/search:8",
				3)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n3"))
			c.after(2 * time.Minute)
			for round := int32(1); round <= 6; round++ {
				c.update(w.pod(2, "n3", crashLoop(78, "Error",
					"bootstrap checks failed: max virtual memory areas "+
						"vm.max_map_count [65530] is too low, increase "+
						"to at least [262144]", round)))
				w.setReady(2)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}
