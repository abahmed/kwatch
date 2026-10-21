package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// advNodeScenarios are adversarial node stories: a coincidence that
// tempts a wrong blame, and failures that move or hide.
func advNodeScenarios() []scenario {
	return []scenario{
		advRolloutThenNodeLost(), advOOMTwoVictims(),
		advCleanDrainQuiet(), advPDBZeroQuiet(),
	}
}

// advRolloutThenNodeLost: catalog finishes a healthy rollout; eight
// seconds later n2 stops reporting and the pods of three workloads on
// it, catalog's new revision among them, go not ready. The node is the
// root. The rollout changed first, but nothing about it failed.
func advRolloutThenNodeLost() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-rollout-then-node-lost",
			Description: "A healthy rollout of one Deployment ends eight " +
				"seconds before a node stops reporting; pods of three " +
				"workloads on the node, including the new revision, go " +
				"not ready.",
			Root: "node//n2", Tier: "page", MaxMessages: 3,
			MustNotBlame: []string{"deployment/shop/catalog",
				"deployment/shop/cart", "deployment/shop/search",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			n2 := c.node("n2", "zone-a")
			c.list(c.node("n1", "zone-a"), n2, c.node("n3", "zone-a"))
			nodes := []string{"n1", "n2", "n3"}
			fleet := nodeFleet(c, "shop",
				[]string{"catalog", "cart", "search"}, nodes)
			catalog := fleet[0]
			c.after(3 * time.Minute)
			old := []*corev1.Pod{catalog.pod(0, "n1"), catalog.pod(1, "n2"),
				catalog.pod(2, "n3")}
			rs := catalog.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image =
					"registry.example.com/catalog:1.1"
			})
			c.update(catalog.deployment)
			c.create(rs)
			for i, node := range nodes {
				c.create(catalog.pod(i, node, startedNow))
				c.remove(old[i])
				c.after(2 * time.Second)
			}
			catalog.setReady(3)
			c.update(catalog.objects())
			c.after(2 * time.Second)
			nodeLose(c, n2)
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(1, "n2", notReady))
				w.setReady(2)
				c.update(w.objects())
			}
			c.after(4 * time.Minute)
		},
	}
}
