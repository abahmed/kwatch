package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// zoneNodesLostStaggered: the three nodes of one zone stop reporting
// minutes apart while the other zone stays healthy. It is one outage of
// the zone, however slowly it spreads: people are paged once.
func zoneNodesLostStaggered() scenario {
	return scenario{
		expect: expectation{
			Name: "zone-nodes-lost-staggered",
			Description: "The three nodes of one zone go down four " +
				"minutes apart while the other zone stays healthy: " +
				"one outage, one page.",
			Root: "zone//zone-a", Tier: "page", MaxMessages: 4,
			MaxPages:     1,
			MustNotBlame: []string{"zone//zone-b", "node//b1", "node//b2"},
			Tail:         duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			zones := map[string]string{"a1": "zone-a", "a2": "zone-a",
				"a3": "zone-a", "b1": "zone-b", "b2": "zone-b"}
			names := []string{"a1", "a2", "a3", "b1", "b2"}
			nodes := map[string]*corev1.Node{}
			for _, name := range names {
				nodes[name] = c.node(name, zones[name])
				c.list(nodes[name])
			}
			fleet := nodeFleet(c, "shop",
				[]string{"cart", "search", "auth"}, names)
			c.after(2 * time.Minute)
			for step, lost := range []string{"a1", "a2", "a3"} {
				nodeLose(c, nodes[lost])
				c.after(40 * time.Second)
				for _, w := range fleet {
					c.update(w.pod(step, lost, notReady))
					w.setReady(int32(len(names) - step - 1))
					c.update(w.objects())
				}
				c.after(4*time.Minute + 20*time.Second)
			}
		},
	}
}
