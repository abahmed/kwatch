package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nodeAgentScenarios are node-level agents that live outside
// kube-system.
func nodeAgentScenarios() []scenario {
	return append([]scenario{agentOutsideKubeSystem()},
		nodeAgentSingleScenarios()...)
}

// agentOutsideKubeSystem: a log shipper DaemonSet of the observability
// namespace runs on every node. Its pod on n1 crash-loops, and replicas
// of two workloads on n1 turn unready while their replicas on n2 are
// fine. The agent is the root, as a kube-system agent would be.
func agentOutsideKubeSystem() scenario {
	return scenario{
		expect: expectation{
			Name: "node-agent-outside-kube-system",
			Description: "A log shipper DaemonSet on every node, outside " +
				"kube-system, crash-loops on one node while pods of two " +
				"workloads beside it turn unready.",
			Root: "daemonset/observability/logship", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "deployment/shop/cart",
				"deployment/shop/search"},
		},
		build: buildAgentOutsideKubeSystem,
	}
}

func buildAgentOutsideKubeSystem(c *cluster) {
	nodes := []string{"n1", "n2"}
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
	apps := []*workload{
		c.deployment("shop", "cart", "registry.example.com/cart:5", 2),
		c.deployment("shop", "search", "registry.example.com/search:9", 2),
	}
	for _, w := range apps {
		c.list(w.objects())
		c.list(w.pod(0, nodes[0]), w.pod(1, nodes[1]))
	}
	c.after(10 * time.Minute)
	setAgentCounts(ds, 2, 1)
	c.update(ds)
	probe := "Readiness probe failed: Get \"http://10.244.1.9:8080/" +
		"ready\": context deadline exceeded (Client.Timeout exceeded " +
		"while awaiting headers)"
	for round := int32(1); round <= 4; round++ {
		c.update(agentPod(c, ds, "n1", crashLoop(1, "Error",
			"failed to open /var/log/pods: permission denied", round*3)))
		for _, w := range apps {
			pod := w.pod(0, nodes[0], notReady)
			c.update(pod)
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", probe,
				"kubelet", round*3))
			w.setReady(1)
			c.update(w.objects())
		}
		c.after(time.Minute)
	}
}
