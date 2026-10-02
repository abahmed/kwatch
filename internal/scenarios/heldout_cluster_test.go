package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// heldoutClusterScenarios are held-out failures of shared services,
// nodes and namespaces. See heldoutLibrary for the rule they follow.
func heldoutClusterScenarios() []scenario {
	return []scenario{
		heldoutNodeDiskPressure(), heldoutNamespaceDNSBlocked(),
		heldoutQuotaOneNamespace(),
	}
}

// heldoutNodeDiskPressure: container logs fill n2's root disk. The
// kubelet reports DiskPressure, taints the node and evicts the pods
// using the most ephemeral storage; their replacements start on n1 and
// n3. The node is the root.
func heldoutNodeDiskPressure() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-node-disk-pressure",
			Description: "A node reports DiskPressure and the kubelet " +
				"evicts pods of two workloads for ephemeral storage; " +
				"replacements start on other nodes.",
			Root: "node//n2", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/media/transcoder",
				"deployment/media/thumbnailer", "zone//zone-b"},
		},
		build: buildHeldoutDiskPressure,
	}
}

func buildHeldoutDiskPressure(c *cluster) {
	n2 := c.node("n2", "zone-b")
	c.list(c.node("n1", "zone-b"), n2, c.node("n3", "zone-b"))
	fleet := nodeFleet(c, "media", []string{"transcoder", "thumbnailer"},
		[]string{"n1", "n2"})
	c.after(2 * time.Minute)
	pressured := last(c, n2)
	setNodeCondition(pressured, corev1.NodeDiskPressure,
		corev1.ConditionTrue, "KubeletHasDiskPressure",
		"kubelet has disk pressure", c.now)
	pressured.Spec.Taints = []corev1.Taint{{
		Key: "node.kubernetes.io/disk-pressure", Effect: "NoSchedule",
	}}
	c.update(pressured)
	c.warn(c.warningEvent(pressured, "Node", "EvictionThresholdMet",
		"Attempting to reclaim ephemeral-storage", "kubelet", 1))
	c.after(30 * time.Second)
	message := "The node was low on resource: ephemeral-storage. " +
		"Threshold quantity: 10%, available: 4%. Container app was " +
		"using 14Gi, request is 0, has larger consumption of " +
		"ephemeral-storage."
	for _, w := range fleet {
		pod := w.pod(1, "n2", evicted(message))
		nodeDisruption(c, pod, "TerminationByKubelet",
			"The node was low on resource: ephemeral-storage.")
		c.update(pod)
		c.warn(c.warningEvent(pod, "Pod", "Evicted", message, "kubelet",
			1))
		w.setReady(1)
		c.update(w.objects())
	}
	c.after(20 * time.Second)
	for _, w := range fleet {
		c.create(w.pod(2, "n3", startedNow))
		w.setReady(2)
		c.update(w.objects())
	}
	c.after(5 * time.Minute)
}

// heldoutNamespaceDNSBlocked: a default-deny egress NetworkPolicy is
// applied to the orders namespace without the usual DNS exception. Pods
// there time out resolving any name, while CoreDNS is healthy, kwatch's
// own lookup succeeds and the shop namespace resolves fine. The policy
// is the root; cluster DNS must never be blamed.
func heldoutNamespaceDNSBlocked() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-namespace-dns-blocked",
			Description: "A default-deny egress NetworkPolicy blocks " +
				"DNS for one namespace; its pods crash on lookups while " +
				"cluster DNS and other namespaces stay healthy.",
			Root: "networkpolicy/orders/default-deny-egress", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"cluster-dns//cluster-dns",
				"deployment/kube-system/coredns", "node//n1", "node//n2"},
		},
		build: buildHeldoutNamespaceDNS,
	}
}

func buildHeldoutNamespaceDNS(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	dns := c.deployment("kube-system", "coredns",
		"registry.k8s.io/coredns/coredns:v1.11.1", 2)
	checkout := c.deployment("orders", "checkout",
		"registry.example.com/checkout:3.0", 2)
	shop := c.deployment("shop", "web", "registry.example.com/web:5", 2)
	for _, w := range []*workload{dns, checkout, shop} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.probe(kube.ClusterDNS, "")
	c.after(2 * time.Minute)
	c.create(&networkingv1.NetworkPolicy{
		ObjectMeta: clusterMeta(c, "orders", "default-deny-egress",
			"netpol"),
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	})
	c.after(20 * time.Second)
	message := "Error: dial tcp: lookup " + c.n("db.orders.svc") +
		".cluster.local on 10.96.0.10:53: read udp 10.244.1.9:52114->" +
		"10.96.0.10:53: i/o timeout"
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(
			checkout.pod(0, "n1", crashLoop(1, "Error", message, restarts)),
			checkout.pod(1, "n2", crashLoop(1, "Error", message, restarts)))
		checkout.setReady(0)
		c.update(checkout.objects())
		c.probe(kube.ClusterDNS, "")
		c.after(45 * time.Second)
	}
}

// heldoutQuotaOneNamespace: the ml team's namespace has a memory quota
// of 32Gi. A training Deployment scales from 3 to 6 replicas of 8Gi
// each; the fourth is forbidden. At the same time a web Deployment in
// another namespace scales out fine on the same nodes. The quota is the
// root.
func heldoutQuotaOneNamespace() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-quota-one-namespace",
			Description: "A Deployment scale-up exceeds its namespace's " +
				"memory ResourceQuota while another namespace scales " +
				"out normally on the same nodes.",
			Root: "resourcequota/ml/team-quota", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"deployment/web/site"},
		},
		build: buildHeldoutQuota,
	}
}

func buildHeldoutQuota(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	train := c.deployment("ml", "trainer", "registry.example.com/train:2", 3)
	site := c.deployment("web", "site", "registry.example.com/site:11", 2)
	c.list(train.objects())
	c.list(train.pod(0, "n1"), train.pod(1, "n2"), train.pod(2, "n1"))
	c.list(site.objects())
	c.list(site.pod(0, "n1"), site.pod(1, "n2"))
	c.list(heldoutMemoryQuota(c, "24Gi"))
	c.after(2 * time.Minute)
	setReplicas(site, 4)
	site.setReady(2)
	c.update(site.objects())
	c.create(site.pod(2, "n1", startedNow), site.pod(3, "n2", startedNow))
	site.setReady(4)
	c.update(site.objects())
	setReplicas(train, 6)
	train.setReady(3)
	c.update(train.objects())
	c.update(heldoutMemoryQuota(c, "32Gi"))
	c.create(train.pod(3, "n2", startedNow))
	for n := int32(1); n <= 5; n++ {
		message := fmt.Sprintf("Error creating: pods \"%s-%c\" is "+
			"forbidden: exceeded quota: %s, requested: requests.memory="+
			"8Gi, used: requests.memory=32Gi, limited: requests.memory="+
			"32Gi", train.replicaSet.Name, 'd'+rune(n),
			c.n("team-quota"))
		c.warn(c.warningEvent(train.replicaSet, "ReplicaSet",
			"FailedCreate", message, "replicaset-controller", n*3))
		c.after(time.Minute)
	}
}

func heldoutMemoryQuota(c *cluster, used string) *corev1.ResourceQuota {
	quota := clusterQuota(c, "0", "0")
	quota.ObjectMeta = clusterMeta(c, "ml", "team-quota", "quota")
	hard := corev1.ResourceList{
		corev1.ResourceRequestsMemory: resource.MustParse("32Gi"),
	}
	quota.Spec.Hard, quota.Status.Hard = hard, hard
	quota.Status.Used = corev1.ResourceList{
		corev1.ResourceRequestsMemory: resource.MustParse(used),
	}
	return quota
}
