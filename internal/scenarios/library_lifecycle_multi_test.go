package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// lifecycleEvictionMessage is the kubelet's eviction reason for a pod of
// container using memory on a node under memory pressure.
func lifecycleEvictionMessage(container string) string {
	return "The node was low on resource: memory. Threshold quantity: " +
		"100Mi, available: 61Mi. Container " + container + " was using " +
		"3Gi, request is 1Gi, has larger consumption of memory."
}

// lifecycleMemoryPressure turns on the node's MemoryPressure condition
// and taint, as the kubelet does when it starts evicting.
func lifecycleMemoryPressure(c *cluster, node *corev1.Node) *corev1.Node {
	pressured := last(c, node)
	setNodeCondition(pressured, corev1.NodeMemoryPressure,
		corev1.ConditionTrue, "KubeletHasInsufficientMemory",
		"kubelet has insufficient memory available", c.now)
	pressured.Spec.Taints = append(pressured.Spec.Taints, corev1.Taint{
		Key: "node.kubernetes.io/memory-pressure", Effect: "NoSchedule",
	})
	return pressured
}

// lifecycleEvict evicts replica i of w from node, with its Evicted event.
func lifecycleEvict(c *cluster, w *workload, i int, node string) {
	message := lifecycleEvictionMessage("app")
	pod := w.pod(i, node, evicted(message))
	c.update(pod)
	c.warn(c.warningEvent(pod, "Pod", "Evicted", message, "kubelet", 1))
}

// twoIndependentProblems: at the same time a node under memory pressure
// evicts batch pods and an unrelated shop release crash-loops on another
// node. Each is its own incident with its own root.
func twoIndependentProblems() scenario {
	return scenario{
		expect: expectation{
			Name: "two-independent-problems",
			Description: "Node n1 comes under memory pressure and evicts " +
				"indexer pods while, a minute later on node n2, a new " +
				"checkout image crash-loops; two unrelated incidents.",
			Root:       "node//n1",
			OtherRoots: []string{"deployment/shop/checkout"},
			Tier:       "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//n2"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			indexer := c.deployment("batch", "indexer",
				"registry.example.com/indexer:3.2", 3)
			c.list(indexer.objects())
			c.list(indexer.pod(0, "n1"), indexer.pod(1, "n1"),
				indexer.pod(2, "n3"))
			checkout := c.deployment("shop", "checkout",
				"registry.example.com/checkout:8.0", 2)
			c.list(checkout.objects())
			c.list(checkout.pod(0, "n2"), checkout.pod(1, "n3"))
			c.after(2 * time.Minute)
			c.update(lifecycleMemoryPressure(c, n1))
			c.after(10 * time.Second)
			lifecycleEvict(c, indexer, 0, "n1")
			lifecycleEvict(c, indexer, 1, "n1")
			indexer.setReady(1)
			c.update(indexer.objects())
			c.after(time.Minute)
			lifecycleBadRelease(c, checkout, "n2")
		},
	}
}

// lifecycleBadRelease rolls w to an image whose new pod on node crashes.
func lifecycleBadRelease(c *cluster, w *workload, node string) {
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image += "1"
	})
	c.update(w.deployment)
	c.create(rs)
	c.create(w.pod(0, node, startedNow, notReady))
	for restarts := int32(1); restarts <= 4; restarts++ {
		c.after(40 * time.Second)
		c.update(w.pod(0, node, startedNow, crashLoop(1, "Error",
			"TypeError: cannot read properties of undefined "+
				"(reading 'currency')", restarts)))
	}
}

// causeRevised: an API pod on n3 is OOM-killed first, which looks like the
// application's own memory use. Minutes later n3 reports memory pressure
// and evicts pods of two workloads: the node was the root all along, and
// the incident's cause must end at the node.
func causeRevised() scenario {
	return scenario{
		expect: expectation{
			Name: "cause-revised",
			Description: "A pod on node n3 is OOM-killed under a generous " +
				"limit; then n3 reports MemoryPressure and evicts pods of " +
				"two workloads whose replicas elsewhere stay healthy.",
			Root: "node//n3", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1", "zone//zone-a"},
		},
		build: func(c *cluster) {
			n3 := c.node("n3", "zone-a")
			c.list(c.node("n1", "zone-a"), n3)
			api := c.deployment("shop", "api",
				"registry.example.com/api:9.1", 2)
			api.rollout(lifecycleMemoryLimit("2Gi"))
			api.setReady(2)
			worker := c.deployment("shop", "worker",
				"registry.example.com/worker:4.4", 2)
			c.list(api.objects())
			c.list(worker.objects())
			c.list(api.pod(0, "n3"), api.pod(1, "n1"),
				worker.pod(0, "n3"), worker.pod(1, "n1"))
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 3; restarts++ {
				c.update(api.pod(0, "n3",
					crashLoop(137, "OOMKilled", "", restarts)))
				c.after(40 * time.Second)
			}
			api.setReady(1)
			c.update(api.objects())
			c.after(2 * time.Minute)
			c.update(lifecycleMemoryPressure(c, n3))
			c.after(15 * time.Second)
			lifecycleEvict(c, api, 0, "n3")
			lifecycleEvict(c, worker, 0, "n3")
			worker.setReady(1)
			c.update(worker.objects())
		},
	}
}
