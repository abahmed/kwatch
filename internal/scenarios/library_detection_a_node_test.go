package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeJoinScenarios are nodes that join a healthy cluster and take a few
// minutes to turn Ready: a node whose network plugin is still starting
// is slow, not lost, until the boot window has passed.
func nodeJoinScenarios() []scenario {
	return []scenario{nodeJoinSlowReady(), nodeJoinNeverReady()}
}

const cniStarting = "container runtime network not ready: " +
	"NetworkReady=false reason:NetworkPluginNotReady message:Network " +
	"plugin returns error: cni plugin not initialized"

// joiningNodeCluster lists a healthy cluster, then a node that joins a
// minute later NotReady while its network plugin starts. It returns the
// new node.
func joiningNodeCluster(c *cluster) *corev1.Node {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:1.0", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(time.Minute)
	node := c.node("n3", "zone-a")
	node.CreationTimestamp = metav1.NewTime(c.now)
	setNodeCondition(node, corev1.NodeReady, corev1.ConditionFalse,
		"KubeletNotReady", cniStarting, c.now)
	c.create(node)
	return node
}

// nodeJoinSlowReady: a new node turns Ready after four minutes. Nothing
// is wrong, so nothing is said.
func nodeJoinSlowReady() scenario {
	return scenario{
		expect: expectation{
			Name: "node-join-slow-ready",
			Description: "A new node joins NotReady while its network " +
				"plugin starts and turns Ready four minutes later.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			node := joiningNodeCluster(c)
			c.after(4 * time.Minute)
			ready := last(c, node)
			setNodeCondition(ready, corev1.NodeReady,
				corev1.ConditionTrue, "KubeletReady",
				"kubelet is posting ready status", c.now)
			c.update(ready)
		},
	}
}

// nodeJoinNeverReady: the twin that never turns Ready. It is reported
// once the boot window has passed, as a node that has not been Ready
// since it joined.
func nodeJoinNeverReady() scenario {
	return scenario{
		expect: expectation{
			Name: "node-join-never-ready",
			Description: "A new node joins NotReady and is still not " +
				"Ready when the boot window ends.",
			Root: "node//n3", Tier: "page", MaxMessages: 2,
			BootHeld: true, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			joiningNodeCluster(c)
			c.after(kube.BootWindow + 5*time.Minute)
		},
	}
}
