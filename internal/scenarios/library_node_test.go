package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nodeScenarios are failures of nodes and zones, and the negative cases
// where a node is healthy or merely cordoned and must not be blamed.
func nodeScenarios() []scenario {
	return []scenario{
		nodeMemoryPressureEviction(), nodeLostNotReady(),
		nodeHealthyAppCrash(), nodeCordonedAppCrash(), nodeZoneFailure(),
		nodeZoneOutageHealthyNodeCrash(),
	}
}

// nodeMemoryPressureEviction: n1 runs low on memory and the kubelet
// evicts the replicas of three workloads there; their replicas on other
// nodes stay healthy. The node is the cause.
func nodeMemoryPressureEviction() scenario {
	return scenario{
		expect: expectation{
			Name: "node-memory-pressure-eviction",
			Description: "A node reports MemoryPressure and the kubelet " +
				"evicts pods of three workloads; their replicas on other " +
				"nodes stay healthy.",
			Root: "node//n1", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/payments", "deployment/shop/cart",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"orders", "payments",
				"cart"}, []string{"n1", "n2"})
			c.after(time.Minute)
			pressured := last(c, n1)
			setNodeCondition(pressured, corev1.NodeMemoryPressure,
				corev1.ConditionTrue, "KubeletHasInsufficientMemory",
				"kubelet has insufficient memory available", c.now)
			pressured.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/memory-pressure", Effect: "NoSchedule",
			}}
			c.update(pressured)
			c.warn(c.warningEvent(pressured, "Node", "EvictionThresholdMet",
				"Attempting to reclaim memory", "kubelet", 1))
			c.after(20 * time.Second)
			for _, w := range fleet {
				pod := w.pod(0, "n1", nodeEvictedForMemory)
				c.update(pod)
				c.warn(c.warningEvent(pod, "Pod", "Evicted",
					pod.Status.Message, "kubelet", 1))
				w.setReady(1)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(15 * time.Second)
			for _, w := range fleet {
				c.create(w.pod(2, "n3", startedNow))
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(5 * time.Minute)
			relieved := last(c, n1)
			setNodeCondition(relieved, corev1.NodeMemoryPressure,
				corev1.ConditionFalse, "KubeletHasSufficientMemory",
				"kubelet has sufficient memory available", c.now)
			relieved.Spec.Taints = nil
			c.update(relieved)
		},
	}
}

// nodeLostNotReady: n1 stops reporting; the node controller marks the
// eight pods on it not ready and, after five minutes, evicts them while
// replacements start elsewhere. The node is the cause.
func nodeLostNotReady() scenario {
	names := make([]string, 0, 8)
	for i := range 8 {
		names = append(names, fmt.Sprintf("svc-%d", i))
	}
	return scenario{
		expect: expectation{
			Name: "node-lost-notready",
			Description: "A node stops reporting (Ready=Unknown); the " +
				"pods of eight workloads on it go not ready and are " +
				"evicted after five minutes.",
			Root: "node//n1", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"zone//zone-a",
				"deployment/apps/svc-0"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "apps", names, []string{"n1", "n2"})
			c.after(time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(1)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(5 * time.Minute)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady, nodeTaintEvicted))
				c.create(w.pod(2, "n3", startedNow))
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
			}
		},
	}
}

// nodeHealthyAppCrash: an application crash-loops on both of its nodes,
// one of which logged an unrelated image garbage-collection warning. The
// application itself is the root; no node may be blamed.
func nodeHealthyAppCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "healthy-node-app-crash",
			Description: "An app crash-loops on two healthy nodes, one " +
				"of which has an unrelated image GC warning.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3.4",
				2)
			healthy := c.deployment("shop", "web",
				"registry.example.com/web:1.9", 1)
			c.list(w.objects())
			c.list(healthy.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), healthy.pod(0, "n1"))
			c.after(time.Minute)
			c.warn(c.warningEvent(n1, "Node", "ImageGCFailed",
				"failed to garbage collect required amount of images. "+
					"Attempted to free 2147483648 bytes, but only found "+
					"0 bytes eligible to free.", "kubelet", 1))
			c.after(30 * time.Second)
			nodeCrashWorkload(c, w, []string{"n1", "n2"},
				"panic: assignment to entry in nil map")
		},
	}
}

// nodeCordonedAppCrash: n1 is cordoned for maintenance; a single-replica
// app already running there starts crash-looping. Cordoning does not
// touch running pods, so the app is the root.
func nodeCordonedAppCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "cordoned-node-app-crash",
			Description: "A node is cordoned; a single-replica app " +
				"running on it crash-loops for its own reasons.",
			Root: "deployment/shop/worker", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "zone//zone-a"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			w := c.deployment("shop", "worker",
				"registry.example.com/worker:5.0", 1)
			other := c.deployment("shop", "web",
				"registry.example.com/web:1.9", 1)
			c.list(w.objects())
			c.list(other.objects())
			c.list(w.pod(0, "n1"), other.pod(0, "n1"))
			c.after(time.Minute)
			cordoned := last(c, n1)
			cordoned.Spec.Unschedulable = true
			cordoned.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
			}}
			c.update(cordoned)
			c.after(90 * time.Second)
			nodeCrashWorkload(c, w, []string{"n1"},
				"Error: queue consumer: unexpected message schema "+
					"version 7")
		},
	}
}

// nodeZoneFailure: both nodes of zone-b stop reporting; every pod there
// goes not ready while the replicas in zone-a and zone-c stay healthy.
// The zone is the cause, not either node alone.
func nodeZoneFailure() scenario {
	return scenario{
		expect: expectation{
			Name: "zone-failure",
			Description: "Every node of one zone stops reporting; " +
				"replicas in the other zones stay healthy.",
			Root: "zone//zone-b", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"zone//zone-a", "zone//zone-c"},
		},
		build: func(c *cluster) {
			zones := map[string]string{"a1": "zone-a", "b1": "zone-b",
				"b2": "zone-b", "c1": "zone-c"}
			lost := []*corev1.Node{}
			for _, name := range []string{"a1", "b1", "b2", "c1"} {
				node := c.node(name, zones[name])
				c.list(node)
				if zones[name] == "zone-b" {
					lost = append(lost, node)
				}
			}
			names := []string{"cart", "search", "catalog", "auth"}
			fleet := nodeFleet(c, "shop", names,
				[]string{"a1", "b1", "c1", "b2"})
			c.after(time.Minute)
			for _, node := range lost {
				nodeLose(c, node)
			}
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(1, "b1", notReady), w.pod(3, "b2", notReady))
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
			}
		},
	}
}

// nodeZoneOutageHealthyNodeCrash: two of zone-a's three nodes stop
// reporting, and a minute later an app whose replicas both run on the
// zone's healthy node n3 starts crash-looping on its own. The zone is
// the outage's root, but not the crash's: the node between the zone
// and the app works.
func nodeZoneOutageHealthyNodeCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "zone-outage-healthy-node-crash",
			Description: "Two nodes of a zone stop reporting while an " +
				"app on the zone's healthy node crash-loops for its " +
				"own reasons.",
			Root:       "zone//zone-a",
			OtherRoots: []string{"deployment/shop/api"},
			Tier:       "page", MaxMessages: 4,
			MustNotBlame: []string{"node//n3", "zone//zone-b"},
		},
		build: func(c *cluster) {
			var lost []*corev1.Node
			for _, name := range []string{"n1", "n2", "n3"} {
				node := c.node(name, "zone-a")
				c.list(node)
				if name != "n3" {
					lost = append(lost, node)
				}
			}
			c.list(c.node("n4", "zone-b"))
			fleet := nodeFleet(c, "shop", []string{"web"},
				[]string{"n1", "n2", "n4"})
			api := c.deployment("shop", "api",
				"registry.example.com/api:3.4", 2)
			c.list(api.objects())
			c.list(api.pod(0, "n3"), api.pod(1, "n3"))
			c.after(time.Minute)
			for _, node := range lost {
				nodeLose(c, node)
			}
			c.after(40 * time.Second)
			c.update(fleet[0].pod(0, "n1", notReady),
				fleet[0].pod(1, "n2", notReady))
			fleet[0].setReady(1)
			c.update(fleet[0].deployment, fleet[0].replicaSet)
			c.after(time.Minute)
			nodeCrashWorkload(c, api, []string{"n3", "n3"},
				"panic: assignment to entry in nil map")
		},
	}
}

// nodeFleet lists healthy Deployments with one replica on each node.
func nodeFleet(
	c *cluster, namespace string, names, nodes []string,
) []*workload {
	fleet := make([]*workload, 0, len(names))
	for _, name := range names {
		w := c.deployment(namespace, name,
			"registry.example.com/"+name+":1.0", int32(len(nodes)))
		c.list(w.objects())
		for i, node := range nodes {
			c.list(w.pod(i, node))
		}
		fleet = append(fleet, w)
	}
	return fleet
}

// nodeLose marks a node unreachable, as the node lifecycle controller
// does when the kubelet stops posting status.
func nodeLose(c *cluster, node *corev1.Node) {
	lost := last(c, node)
	setNodeCondition(lost, corev1.NodeReady, corev1.ConditionUnknown,
		"NodeStatusUnknown", "Kubelet stopped posting node status.", c.now)
	lost.Spec.Taints = []corev1.Taint{
		{Key: "node.kubernetes.io/unreachable", Effect: "NoSchedule"},
		{Key: "node.kubernetes.io/unreachable", Effect: "NoExecute"},
	}
	c.update(lost)
}

// nodeCrashWorkload makes the workload's replicas on nodes crash-loop,
// recording two restarts rounds as the kubelet backs off.
func nodeCrashWorkload(c *cluster, w *workload, nodes []string,
	message string) {
	for _, restarts := range []int32{3, 5} {
		for i, node := range nodes {
			c.update(w.pod(i, node, crashLoop(2, "Error", message, restarts)))
		}
		w.setReady(0)
		c.update(w.deployment, w.replicaSet)
		c.after(2 * time.Minute)
	}
}

// nodeEvictedForMemory is a kubelet node-pressure eviction.
func nodeEvictedForMemory(c *cluster, pod *corev1.Pod) {
	evicted("The node was low on resource: memory. Threshold quantity: "+
		"100Mi, available: 81Mi. Container app was using 1843Mi, "+
		"request is 512Mi, has larger consumption of memory.")(c, pod)
	nodeDisruption(c, pod, "TerminationByKubelet",
		"The node was low on resource: memory.")
}

// nodeTaintEvicted is the taint manager deleting a pod from an
// unreachable node; the pod stays Terminating as the kubelet is gone.
func nodeTaintEvicted(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	pod.DeletionTimestamp = &now
	nodeDisruption(c, pod, "DeletionByTaintManager",
		"Taint manager: deleting due to NoExecute taint")
}

func nodeDisruption(c *cluster, pod *corev1.Pod, reason, message string) {
	pod.Status.Conditions = append(pod.Status.Conditions,
		corev1.PodCondition{
			Type: "DisruptionTarget", Status: corev1.ConditionTrue,
			Reason: reason, Message: message,
			LastTransitionTime: metav1.NewTime(c.now),
		})
}
