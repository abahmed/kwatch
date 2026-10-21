package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// kubeletNoticeScenarios are the Warning notices the kubelet writes on a
// healthy start: nothing is wrong, so nothing is said.
func kubeletNoticeScenarios() []scenario {
	return []scenario{nodeJoinKubeletNotices(), podDNSSearchLineNotice()}
}

// nodeJoinKubeletNotices: a node joins and its kubelet writes the
// InvalidDiskCapacity and Rebooted notices before the node turns Ready.
func nodeJoinKubeletNotices() scenario {
	return scenario{
		expect: expectation{
			Name: "node-join-kubelet-notices",
			Description: "A node joins; before it turns Ready its " +
				"kubelet records InvalidDiskCapacity and Rebooted.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.after(time.Hour)
			node := c.node("n2", "zone-a")
			node.CreationTimestamp = metav1.NewTime(c.now)
			setNodeCondition(node, corev1.NodeReady, corev1.ConditionFalse,
				"KubeletNotReady", "container runtime not ready", c.now)
			c.create(node)
			c.warn(c.warningEvent(node, "Node", "InvalidDiskCapacity",
				"invalid capacity 0 on image filesystem", "kubelet", 1))
			c.warn(c.warningEvent(node, "Node", "Rebooted",
				"Node n2 has been rebooted, boot id: 7f3a", "kubelet", 1))
			c.after(90 * time.Second)
			ready := last(c, node)
			setNodeCondition(ready, corev1.NodeReady,
				corev1.ConditionTrue, "KubeletReady",
				"kubelet is posting ready status", c.now)
			c.update(ready)
		},
	}
}

// podDNSSearchLineNotice: the kubelet warns that the node's resolv.conf
// has a search line longer than the limit for every pod it starts; the
// pods run and serve.
func podDNSSearchLineNotice() scenario {
	return scenario{
		expect: expectation{
			Name: "pod-dns-search-line-notice",
			Description: "The kubelet warns about an over-long DNS " +
				"search line for each of three healthy pods.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api",
				"registry.example.com/api:1.0", 3)
			c.list(w.objects())
			pods := []*corev1.Pod{
				w.pod(0, "n1"), w.pod(1, "n1"), w.pod(2, "n1")}
			c.list(pods[0], pods[1], pods[2])
			c.after(time.Hour)
			for _, pod := range pods {
				c.warn(c.warningEvent(pod, "Pod", "DNSConfigForming",
					"Search Line limits were exceeded, some search "+
						"paths have been omitted, the applied search "+
						"line is: shop.svc.cluster.local svc.cluster."+
						"local cluster.local ec2.internal", "kubelet", 1))
			}
			c.after(5 * time.Minute)
		},
	}
}
