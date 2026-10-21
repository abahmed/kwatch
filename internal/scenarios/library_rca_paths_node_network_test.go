package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rcaNodeNetworkScenarios are faults local to one node that show only as
// connection errors while the node reports Ready.
func rcaNodeNetworkScenarios() []scenario {
	return []scenario{
		nodeLocalNetwork(), nodeLocalUnmodelledAddress(),
		kubeProxyOneNode(),
	}
}

const rcaCacheAddress = "10.96.40.12"

// rcaCacheFleet lists a cache and three three-replica workloads, one
// replica per node, that all call it; it returns the three. A ClusterIP
// of "" leaves the dialed address unknown to the model.
func rcaCacheFleet(c *cluster, clusterIP string) []*workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
		c.node("n3", "zone-a"))
	cache := c.deployment("shop", "cache", "registry.example.com/cache:7", 1)
	c.list(cache.objects())
	c.list(cache.pod(0, "n1"))
	svc := clusterService(c, "shop", "cache", 6379)
	svc.Spec.ClusterIP = clusterIP
	c.list(svc, clusterSlice(c, "shop", "cache", cache.pod(0, "n1")))
	return nodeFleet(c, "shop", []string{"web", "api", "worker"},
		[]string{"n1", "n2", "n3"})
}

// rcaBrokenNodeRounds makes the replicas on n3 fail: web's go unready
// on a probe timeout, the others crash dialing the cache.
func rcaBrokenNodeRounds(c *cluster, fleet []*workload, rounds int32) {
	timeout := "dial tcp " + rcaCacheAddress + ":6379: i/o timeout"
	began := c.now
	probe := "Readiness probe failed: Get \"http://10.244.2.14:8080/" +
		"healthz\": context deadline exceeded"
	for round := int32(1); round <= rounds; round++ {
		web := fleet[0].pod(2, "n3", notReadySince(began))
		c.update(web)
		c.warn(c.warningEvent(web, "Pod", "Unhealthy", probe, "kubelet",
			round*3))
		fleet[0].setReady(2)
		c.update(fleet[0].objects())
		for _, w := range fleet[1:] {
			c.update(w.pod(2, "n3", crashLoop(1, "Error", timeout, round)))
			w.setReady(2)
			c.update(w.objects())
		}
		c.after(time.Minute)
	}
}

// nodeLocalNetwork: n3 reports Ready but its pods cannot reach anything;
// three workloads' replicas there time out dialing a ClusterIP while the
// same workloads' replicas on n1 and n2 reach the same Service.
func nodeLocalNetwork() scenario {
	return scenario{
		expect: expectation{
			Name: "node-local-network-fault",
			Description: "One node's pods time out on every connection " +
				"while the node reports Ready and replicas of the " +
				"same workloads on other nodes work.",
			Root: "node//n3", Tier: "notify", MaxMessages: 3,
			Ambiguous: true,
			MustNotBlame: []string{"service/shop/cache",
				"deployment/shop/cache", "cluster-dns//cluster-dns",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			fleet := rcaCacheFleet(c, rcaCacheAddress)
			c.after(3 * time.Minute)
			rcaBrokenNodeRounds(c, fleet, 5)
		},
	}
}

// nodeLocalUnmodelledAddress: the same story, but the address the pods
// dial belongs to no Service or pod kwatch holds.
func nodeLocalUnmodelledAddress() scenario {
	return scenario{
		expect: expectation{
			Name: "node-local-unmodelled-address",
			Description: "One node's pods time out dialing an address " +
				"kwatch does not know, while replicas on other nodes " +
				"work.",
			Root: "node//n3", Tier: "notify", MaxMessages: 3,
			Ambiguous: true,
			MustNotBlame: []string{
				"external-endpoint//" + rcaCacheAddress + ":6379",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			fleet := rcaCacheFleet(c, "")
			c.after(3 * time.Minute)
			rcaBrokenNodeRounds(c, fleet, 5)
		},
	}
}

// kubeProxyOneNode: the same symptoms, and kube-proxy on n3 crash-loops
// with an iptables error. One incident, rooted at the agent that says
// what is wrong, not at the node and not at the Service.
func kubeProxyOneNode() scenario {
	return scenario{
		expect: expectation{
			Name: "kube-proxy-one-node-crash",
			Description: "kube-proxy crash-loops on one node; pods " +
				"there time out on Service addresses while other " +
				"nodes work.",
			Root: "daemonset/kube-system/kube-proxy", Tier: "notify",
			MaxMessages: 4,
			MustNotBlame: []string{"service/shop/cache",
				"deployment/shop/cache", "cluster-dns//cluster-dns",
				"deployment/shop/web", "deployment/shop/api",
				"deployment/shop/worker", "node//n3"},
		},
		build: func(c *cluster) {
			fleet := rcaCacheFleet(c, rcaCacheAddress)
			ds := rcaAgent(c, "kube-system", "kube-proxy",
				[]string{"n1", "n2", "n3"})
			c.after(3 * time.Minute)
			crash := "E1020 10:03:11.482113 1 proxier.go:1494] \"Failed " +
				"to execute iptables-restore\" err=\"exit status 4 " +
				"(iptables-restore: line 31 failed)\""
			for round := int32(1); round <= 5; round++ {
				c.update(agentPod(c, ds, "n3",
					crashLoop(1, "Error", crash, round)))
				setAgentCounts(ds, 3, 2)
				c.update(ds)
				rcaBrokenNodeRounds(c, fleet, 1)
			}
		},
	}
}

// rcaAgent lists a DaemonSet with a healthy pod on each node.
func rcaAgent(
	c *cluster, namespace, name string, nodes []string,
) *appsv1.DaemonSet {
	labels := map[string]string{"k8s-app": name}
	ds := &appsv1.DaemonSet{
		ObjectMeta: c.meta(namespace, name),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels,
				"registry.k8s.io/"+name+":v1.30.2"),
		},
	}
	ds.Generation, ds.Status.ObservedGeneration = 1, 1
	setAgentCounts(ds, int32(len(nodes)), int32(len(nodes)))
	c.list(ds)
	for _, node := range nodes {
		c.list(agentPod(c, ds, node))
	}
	return ds
}
