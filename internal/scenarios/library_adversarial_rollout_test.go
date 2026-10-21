package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// advSecretInvisible: a Secret the app reads is rotated and the app
// crash-loops with an error that names nothing. kwatch has no
// permission to read Secrets, so it cannot know whether one changed.
// Unknown, stated as unknown; never "its configuration is unchanged".
func advSecretInvisible() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-secret-rotated-but-invisible",
			Description: "A crash follows a Secret rotation kwatch " +
				"cannot see because it may not list Secrets.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Hidden:       []string{"secret"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			api := c.deployment("shop", "api", "registry.example.com/api:4.2", 2)
			configTemplate(api, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{configSecretEnv(
					"API_KEY", c.n("api-keys"), "key")}
			})
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(5 * time.Minute)
			advLogCrash(c, api, "level=fatal msg=\"startup check failed\"", 5)
		},
	}
}

// advRolloutInFlightNodeLost: catalog's rollout is twenty seconds old
// when n2 stops reporting; its new pod on n2 is still creating and its
// other new pods are not yet ready. The unfinished rollout looks
// stalled, but the stall is the node.
func advRolloutInFlightNodeLost() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-rollout-in-flight-node-lost",
			Description: "A node stops reporting while a rollout that " +
				"has a new pod on it is still in flight.",
			Root: "node//n2", Tier: "page", MaxMessages: 4, MaxPages: 1,
			MustNotBlame: []string{"deployment/shop/catalog",
				"deployment/shop/cart", "zone//zone-a"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			n2 := c.node("n2", "zone-a")
			c.list(c.node("n1", "zone-a"), n2, c.node("n3", "zone-a"))
			nodes := []string{"n1", "n2", "n3"}
			fleet := nodeFleet(c, "shop", []string{"catalog", "cart"}, nodes)
			catalog := fleet[0]
			c.after(3 * time.Minute)
			rs := catalog.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/catalog:1.1"
			})
			catalog.setReady(3)
			c.update(catalog.deployment)
			c.create(rs)
			c.create(catalog.pod(0, "n1", startedNow, notReady))
			c.after(20 * time.Second)
			c.create(catalog.pod(1, "n2", startedNow, creating))
			nodeLose(c, n2)
			c.after(40 * time.Second)
			c.update(fleet[1].pod(1, "n2", notReady))
			fleet[1].setReady(2)
			c.update(fleet[1].objects())
			c.update(catalog.pod(0, "n1", startedNow))
			c.after(5 * time.Minute)
		},
	}
}

// advNodeBlipThenBadRollout: n2 drops out for ninety seconds and comes
// back; just after, a new image of api crash-loops on every node. The
// blip is over and healed; the release is the fault.
func advNodeBlipThenBadRollout() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-node-blip-then-bad-rollout",
			Description: "A node is unreachable for ninety seconds and " +
				"recovers; right after, a bad release crash-loops.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			// The blip is a real, short incident of its own; its page is
			// heard before the release's message.
			Unrelated: []string{"node//n2"},
			MustNotBlame: []string{"node//n1", "node//n3",
				"zone//zone-a"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			n2 := c.node("n2", "zone-a")
			c.list(c.node("n1", "zone-a"), n2, c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"api", "cart"},
				[]string{"n1", "n2", "n3"})
			api := fleet[0]
			c.after(3 * time.Minute)
			nodeLose(c, n2)
			c.after(40 * time.Second)
			c.update(api.pod(1, "n2", notReady))
			c.after(50 * time.Second)
			nodeRecover(c, n2)
			c.update(api.pod(1, "n2"))
			c.after(30 * time.Second)
			rs := api.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/api:2.0"
			})
			c.update(api.deployment)
			c.create(rs)
			crash := "panic: assignment to entry in nil map"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(api.pod(0, "n1", startedNow, crashLoop(2, "Error",
					crash, restarts)), api.pod(1, "n2", startedNow,
					crashLoop(2, "Error", crash, restarts)),
					api.pod(2, "n3", startedNow, crashLoop(2, "Error",
						crash, restarts)))
				c.after(time.Minute)
			}
		},
	}
}

// advTwoCausesLong is advTwoCauses watched for six hours, long after
// the changes left the thirty-minute causal window.
func advTwoCausesLong() scenario {
	s := advTwoCauses()
	s.expect.Name = "adv-two-causes-six-hours"
	s.expect.Tail = duration(6 * time.Hour)
	s.expect.MaxMessages = 3
	s.expect.Description += " Watched for six hours."
	return s
}
