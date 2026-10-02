package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// workloadScenarios are failures of one application: its release, its
// resources or its own code.
func workloadScenarios() []scenario {
	return []scenario{badRollout()}
}

// badRollout: a new image crash-loops while the previous revision keeps
// serving. The rollout is the cause.
func badRollout() scenario {
	return scenario{
		expect: expectation{
			Name: "bad-rollout",
			Description: "A Deployment rolls out a new image whose pods " +
				"crash-loop; pods of the previous revision stay healthy.",
			Root: "deployment/shop/payments", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "payments",
				"registry.example.com/payments:2.2", 2)
			c.list(w.objects())
			old := w.pod(0, "n1")
			c.list(old, w.pod(1, "n2"))
			c.after(70 * time.Second)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/payments:2.3"
			})
			c.update(w.deployment)
			c.create(rs)
			c.create(w.pod(0, "n1", startedNow, notReady))
			c.after(40 * time.Second)
			c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
				"panic: missing key DB_PASSWORD_V2", 3)))
			c.after(2 * time.Minute)
			c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
				"panic: missing key DB_PASSWORD_V2", 5)))
		},
	}
}
