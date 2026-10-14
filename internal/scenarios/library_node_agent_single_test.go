package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nodeAgentSingleScenarios are a node agent and one workload beside it.
func nodeAgentSingleScenarios() []scenario {
	return []scenario{agentBesideLoneWorkload(), agentHealthyAppCrashes()}
}

// logshipAgent lists two nodes and a log shipper DaemonSet with a pod on
// each, and returns the DaemonSet.
func logshipAgent(c *cluster) *appsv1.DaemonSet {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	labels := map[string]string{"app": "logship"}
	ds := &appsv1.DaemonSet{
		ObjectMeta: c.meta("observability", "logship"),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/logship:3"),
		},
	}
	ds.Generation, ds.Status.ObservedGeneration = 1, 1
	setAgentCounts(ds, 2, 2)
	c.list(ds, agentPod(c, ds, "n1"), agentPod(c, ds, "n2"))
	return ds
}

// agentBesideLoneWorkload: the log shipper crash-loops on n1, where one
// workload, cart, is the only application pod. Cart's readiness probe
// times out; search on n2 is fine. The shipper's error ("permission
// denied") has nothing to do with the probe, and one workload is no
// evidence of a shared fault, so the agent is not blamed for cart.
func agentBesideLoneWorkload() scenario {
	return scenario{
		expect: expectation{
			Name: "node-agent-beside-lone-workload",
			Description: "A log shipper DaemonSet crash-loops on a node " +
				"whose only application pod, a single workload, times " +
				"out its probe: two unrelated faults.",
			Root:       "daemonset/observability/logship",
			OtherRoots: []string{"deployment/shop/cart"},
			Tier:       "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			ds := logshipAgent(c)
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:5", 1)
			search := c.deployment("shop", "search",
				"registry.example.com/search:9", 1)
			c.list(cart.objects())
			c.list(cart.pod(0, "n1"))
			c.list(search.objects())
			c.list(search.pod(0, "n2"))
			c.after(10 * time.Minute)
			setAgentCounts(ds, 2, 1)
			c.update(ds)
			probe := "Readiness probe failed: Get \"http://10.244.1.9:" +
				"8080/ready\": context deadline exceeded"
			for round := int32(1); round <= 4; round++ {
				c.update(agentPod(c, ds, "n1", crashLoop(1, "Error",
					"failed to open /var/log/pods: permission denied",
					round*3)))
				pod := cart.pod(0, "n1", notReady)
				c.update(pod)
				c.warn(c.warningEvent(pod, "Pod", "Unhealthy", probe,
					"kubelet", round*3))
				cart.setReady(0)
				c.update(cart.objects())
				c.after(time.Minute)
			}
			c.after(5 * time.Minute)
			setAgentCounts(ds, 2, 2)
			c.update(ds, agentPod(c, ds, "n1"))
			c.update(cart.pod(0, "n1"))
			cart.setReady(1)
			c.update(cart.objects())
			c.after(10 * time.Minute)
		},
	}
}

// agentHealthyAppCrashes: one application crashes on a node that also
// runs a healthy agent and another healthy workload. The agent is not
// failing, so the crashing app is its own root and the agent is not
// blamed.
func agentHealthyAppCrashes() scenario {
	return scenario{
		expect: expectation{
			Name: "node-agent-healthy-app-crashes",
			Description: "One application crash-loops on a node whose " +
				"log shipper agent is healthy.",
			Root: "deployment/shop/cart", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"daemonset/observability/logship",
				"node//n1"},
		},
		build: func(c *cluster) {
			logshipAgent(c)
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:5", 1)
			search := c.deployment("shop", "search",
				"registry.example.com/search:9", 1)
			c.list(cart.objects())
			c.list(cart.pod(0, "n1"))
			c.list(search.objects())
			c.list(search.pod(0, "n1"))
			c.after(10 * time.Minute)
			for round := int32(1); round <= 4; round++ {
				c.update(cart.pod(0, "n1", crashLoop(1, "Error",
					"panic: nil pointer dereference", round*3)))
				cart.setReady(0)
				c.update(cart.objects())
				c.after(time.Minute)
			}
		},
	}
}
