package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// sandboxRefusedMessage is what the kubelet quotes while the network
// plugin of a node that has just joined is not answering yet.
const sandboxRefusedMessage = "Failed to create pod sandbox: rpc error: " +
	"code = Unknown desc = failed to setup network for sandbox \"3f9c\": " +
	"plugin type=\"aws-cni\" name=\"aws-cni\" failed (add): add cmd: " +
	"Error received from AddNetwork gRPC call: rpc error: code = " +
	"Unavailable desc = connection error: dial tcp 127.0.0.1:50051: " +
	"connect: connection refused"

// sandboxBlipScenarios are replacement pods whose network setup fails.
func sandboxBlipScenarios() []scenario {
	return []scenario{nodeReplaceSandboxBlip(), nodeReplaceSandboxStuck()}
}

// nodeReplaceSandboxBlip: a node of a four-node pool is terminated and
// a new one joins. Three single-replica Deployments lose their pod; the
// replacements are bound to the new node and cannot set up their
// sandbox for about two minutes while the node's network plugin
// starts, then all become ready. Nothing needs a person.
func nodeReplaceSandboxBlip() scenario {
	return scenario{
		expect: expectation{
			Name: "node-replace-sandbox-blip",
			Description: "A node is replaced; the replacement pods of " +
				"three single-replica Deployments fail sandbox network " +
				"setup for two minutes, then start.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildSandboxReplace(c, 90*time.Second) },
	}
}

// nodeReplaceSandboxStuck: the same replacement, but the network setup
// never recovers. The single-replica services stay down for more than
// ten minutes, so someone must be told.
func nodeReplaceSandboxStuck() scenario {
	return scenario{
		expect: expectation{
			Name: "node-replace-sandbox-stuck",
			Description: "A node is replaced; the replacement pods of " +
				"three single-replica Deployments keep failing sandbox " +
				"network setup and never start.",
			Root: "node//n3", Tier: "page", MaxMessages: 2,
			BootHeld: true, Tail: duration(5 * time.Minute),
		},
		build: func(c *cluster) { buildSandboxReplace(c, 0) },
	}
}

// sandboxServices are the single-replica Deployments of the scenarios.
var sandboxServices = []string{"fleet", "storage", "support"}

// buildSandboxReplace terminates node n3 of a four-node pool, joins n5
// and replaces the pods of three single-replica Deployments, each behind
// a LoadBalancer Service. They retry their sandbox until heal; a zero
// heal never.
func buildSandboxReplace(c *cluster, heal time.Duration) {
	var works []*workload
	for _, name := range []string{"n1", "n2", "n3", "n4"} {
		c.list(workerNode(c, name))
	}
	for _, name := range sandboxServices {
		w := c.deployment("shop", name, "registry.example.com/"+name+":1", 1)
		service := clusterService(c, "shop", name, 8080)
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		service.Status.LoadBalancer.Ingress =
			[]corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
		c.list(w.objects())
		c.list(w.pod(0, "n3"), service,
			trafficSlice(c, name, w.pod(0, "n3")))
		works = append(works, w)
	}
	c.after(10 * time.Minute)
	c.remove(last(c, workerNode(c, "n3")))
	born := c.now
	n5 := workerNode(c, "n5")
	n5.CreationTimestamp = metav1.NewTime(c.now)
	c.create(n5)
	for i, w := range works {
		c.remove(last(c, w.pod(0, "n3")))
		c.create(w.pod(1, "n5", createdAt(born), creating))
		w.setReady(0)
		c.update(w.objects())
		c.update(trafficUnreadySlice(c, sandboxServices[i],
			w.pod(1, "n5", createdAt(born), creating)))
	}
	sandboxRetries(c, works, born, 3, 25*time.Second)
	if heal == 0 {
		// The kubelet keeps backing off for as long as it fails.
		sandboxRetries(c, works, born, 8, 2*time.Minute)
		return
	}
	c.after(heal)
	for i, w := range works {
		c.update(w.pod(1, "n5", startedNow, createdAt(born)))
		w.setReady(1)
		c.update(w.objects())
		c.update(trafficSlice(c, sandboxServices[i],
			w.pod(1, "n5", startedNow, createdAt(born))))
	}
}

// workerNode is an old node of the "workers" pool.
func workerNode(c *cluster, name string) *corev1.Node {
	node := c.node(name, "zone-a")
	node.Labels["karpenter.sh/nodepool"] = bootPool
	return node
}

// sandboxRetries repeats the sandbox failure of every replacement pod,
// times times, one gap apart.
func sandboxRetries(
	c *cluster, works []*workload, born time.Time, times int,
	gap time.Duration,
) {
	for range times {
		c.after(gap)
		for _, w := range works {
			pod := w.pod(1, "n5", createdAt(born), creating)
			event := c.warningEvent(pod, "Pod", "FailedCreatePodSandBox",
				sandboxRefusedMessage, "kubelet", 2)
			event.InvolvedObject.UID = pod.UID
			c.warn(event)
		}
	}
}
