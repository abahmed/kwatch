package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// nodeCNINotReady: a node goes NotReady because its network plugin is
// not initialized, and its kubelet reports a failed image GC.
// The note quotes the kubelet's own words and nothing else.
func nodeCNINotReady() scenario {
	return scenario{
		expect: expectation{
			Name: "node-cni-not-ready",
			Description: "A node stays NotReady because the container " +
				"runtime network is not ready; the message quotes the " +
				"cause and the failed image GC.",
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
			down := last(c, n1)
			setNodeCondition(down, corev1.NodeReady,
				corev1.ConditionFalse, "KubeletNotReady",
				"container runtime network not ready: NetworkReady=false "+
					"reason:NetworkPluginNotReady message:Network plugin "+
					"returns error: cni plugin not initialized", c.now)
			c.update(down)
			c.warn(c.warningEvent(down, "Node", "ImageGCFailed",
				"failed to get imageFs info: unable to find data in "+
					"memory cache", "kubelet", 1))
			c.after(40 * time.Second)
			c.update(w.pod(0, "n1", notReady))
			w.setReady(1)
			c.update(w.deployment, w.replicaSet)
			c.after(5 * time.Minute)
		},
	}
}
