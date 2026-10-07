package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// nodeFlapScenarios are nodes whose Ready condition changes: brief
// blips that must stay quiet, and a node that really stays down.
func nodeFlapScenarios() []scenario {
	return []scenario{nodeReadyBlips(), nodeStaysDown(),
		nodeCNINotReady()}
}

// setNodeReady flips the Ready condition of node and delivers it.
func setNodeReady(c *cluster, node *corev1.Node, ready bool) {
	changed := last(c, node)
	if ready {
		setNodeCondition(changed, corev1.NodeReady, corev1.ConditionTrue,
			"KubeletReady", "kubelet is posting ready status", c.now)
	} else {
		setNodeCondition(changed, corev1.NodeReady, corev1.ConditionFalse,
			"KubeletNotReady", "container runtime is restarting", c.now)
	}
	c.update(changed)
}

// nodeReadyBlips: a node flips NotReady and Ready three times in four
// minutes, each NotReady shorter than a minute, while its pods keep
// running. Nothing is worth a message.
func nodeReadyBlips() scenario {
	return scenario{
		expect: expectation{
			Name: "node-ready-blips",
			Description: "A node flaps NotReady and Ready three times " +
				"in four minutes, each NotReady under a minute.",
			Quiet: true,
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1)
			w := c.deployment("shop", "api", "registry.example.com/api:3", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(time.Minute)
			for range 3 {
				setNodeReady(c, n1, false)
				c.after(50 * time.Second)
				setNodeReady(c, n1, true)
				c.after(30 * time.Second)
			}
		},
	}
}

// nodeStaysDown: the kubelet of a node stays NotReady for five
// minutes and its pod goes not ready. The node is the cause.
func nodeStaysDown() scenario {
	return scenario{
		expect: expectation{
			Name: "node-stays-down",
			Description: "A node stays NotReady for five minutes and " +
				"its pod goes not ready.",
			Root: "node//n1", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(time.Minute)
			setNodeReady(c, n1, false)
			c.after(40 * time.Second)
			c.update(w.pod(0, "n1", notReady))
			w.setReady(1)
			c.update(w.deployment, w.replicaSet)
			c.after(5 * time.Minute)
		},
	}
}
