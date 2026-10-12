package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// nodeDegradedScenarios are nodes that still serve but struggle, while
// the pods placed on them turn unready. There is no labelled scenario
// for a slow kubelet pod relist: the detector waits five minutes before
// it reports one, which with the notify settle is beyond the five-minute
// first-message gate by design; the node-degraded row is covered by its
// fixtures in explain.
func nodeDegradedScenarios() []scenario {
	return []scenario{nodeConditionTrue()}
}

// nodeConditionTrue: a node reports a condition kwatch does not read
// elsewhere as True; pods of two workloads on it turn unready. The
// condition is the cause, with its type in the message.
func nodeConditionTrue() scenario {
	return scenario{
		expect: expectation{
			Name: "node-condition-true-pods-unready",
			Description: "A node reports a custom condition as True and " +
				"pods of two workloads on it turn unready.",
			Root: "node//n1", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/cart",
				"deployment/shop/search", "node//n2"},
		},
		build: func(c *cluster) {
			buildDegradedNode(c, func(c *cluster) {
				node := last(c, c.node("n1", "zone-a"))
				setNodeCondition(node, "FilesystemReadOnly",
					corev1.ConditionTrue, "FilesystemIsReadOnly",
					"filesystem is read-only", c.now)
				c.update(node)
			})
		},
	}
}

// buildDegradedNode lists two nodes and two workloads with a replica on
// each, strains n1 once with degrade, and the replicas on n1 fail their
// readiness probes at the same time.
func buildDegradedNode(c *cluster, degrade func(c *cluster)) {
	nodes := []string{"n1", "n2"}
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	apps := []*workload{
		c.deployment("shop", "cart", "registry.example.com/cart:5", 2),
		c.deployment("shop", "search", "registry.example.com/search:9", 2),
	}
	for _, w := range apps {
		c.list(w.objects())
		c.list(w.pod(0, nodes[0]), w.pod(1, nodes[1]))
	}
	degrade(c)
	since := c.now
	probe := "Readiness probe failed: Get \"http://10.244.1.9:8080/" +
		"ready\": context deadline exceeded (Client.Timeout exceeded " +
		"while awaiting headers)"
	for round := int32(1); round <= 4; round++ {
		for _, w := range apps {
			pod := w.pod(0, nodes[0], notReadySince(since))
			c.update(pod)
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", probe,
				"kubelet", round*3))
			w.setReady(1)
			c.update(w.objects())
		}
		c.after(time.Minute)
	}
}
