package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// gapNodeAScenarios are nodes that fail for a resource or a runtime
// reason other than memory, disk or network.
func gapNodeAScenarios() []scenario {
	return []scenario{
		gapPIDPressure(), gapPLEGNotHealthy(), gapMaxPodsReached(),
	}
}

// gapNodeFleet lists three two-replica workloads, one pod on each of n1
// and n2, with n3 as the spare node.
func gapNodeFleet(c *cluster) (*corev1.Node, []*workload) {
	n1 := c.node("n1", "zone-a")
	c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
	return n1, nodeFleet(c, "shop", []string{"orders", "payments", "cart"},
		[]string{"n1", "n2"})
}

// gapPIDPressure: the kubelet of n1 reports PIDPressure; pods there
// cannot fork and crash at start while their replicas on n2 run.
func gapPIDPressure() scenario {
	return scenario{
		expect: expectation{
			Name: "node-pid-pressure",
			Description: "A node reports PIDPressure; pods on it fail " +
				"with \"resource temporarily unavailable\" on fork, " +
				"their replicas on another node are healthy.",
			Root: "node//n1", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/payments", "deployment/shop/cart"},
		},
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(time.Minute)
			pressured := last(c, n1)
			setNodeCondition(pressured, corev1.NodePIDPressure,
				corev1.ConditionTrue, "KubeletHasInsufficientPID",
				"kubelet has insufficient PID available", c.now)
			pressured.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/pid-pressure", Effect: "NoSchedule",
			}}
			c.update(pressured)
			c.warn(c.warningEvent(pressured, "Node", "EvictionThresholdMet",
				"Attempting to reclaim pids", "kubelet", 1))
			message := "runtime/cgo: pthread_create failed: Resource " +
				"temporarily unavailable"
			for _, restarts := range []int32{2, 4, 6} {
				c.after(time.Minute)
				for _, w := range fleet {
					c.update(w.pod(0, "n1", crashLoop(2, "Error", message,
						restarts)))
					w.setReady(1)
					c.update(w.objects())
				}
			}
		},
	}
}

// gapPLEGNotHealthy: the container runtime on n1 stops; the kubelet
// reports the node NotReady with a PLEG message and the pods on it go
// unready. The node is the cause, not any one workload.
func gapPLEGNotHealthy() scenario {
	return scenario{
		expect: expectation{
			Name: "node-pleg-not-healthy",
			Description: "A node's container runtime stops; its kubelet " +
				"reports NotReady with \"PLEG is not healthy\" and the " +
				"pods on it go unready.",
			Root: "node//n1", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/payments", "deployment/shop/cart",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(time.Minute)
			down := last(c, n1)
			setNodeCondition(down, corev1.NodeReady, corev1.ConditionFalse,
				"KubeletNotReady", "PLEG is not healthy: pleg was last "+
					"seen active 4m31.2s ago; threshold is 3m0s", c.now)
			down.Spec.Taints = []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"},
				{Key: "node.kubernetes.io/not-ready", Effect: "NoExecute"},
			}
			c.update(down)
			c.warn(c.warningEvent(down, "Node", "ContainerGCFailed",
				"rpc error: code = Unavailable desc = connection error: "+
					"desc = \"transport: Error while dialing dial unix "+
					"/run/containerd/containerd.sock: connect: no such "+
					"file or directory\"", "kubelet", 4))
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(5 * time.Minute)
		},
	}
}

// gapMaxPodsReached: three small nodes each run as many pods as their
// kubelet allows; the replicas of a new Deployment stay Pending on
// "Too many pods". The pod limit is the cause, not the new Deployment.
func gapMaxPodsReached() scenario {
	return scenario{
		expect: expectation{
			Name: "node-max-pods-reached",
			Description: "Every node already runs its maximum number of " +
				"pods; a new Deployment's replicas are Pending with " +
				"\"Too many pods\" on all nodes.",
			Root: "scheduling//Too many pods", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/web",
				"scheduling//Insufficient cpu"},
		},
		build: func(c *cluster) {
			names := []string{"n1", "n2", "n3"}
			for _, name := range names {
				node := c.node(name, "zone-a")
				node.Status.Allocatable[corev1.ResourcePods] =
					resource.MustParse("6")
				c.list(node)
			}
			fill := c.deployment("batch", "filler",
				"registry.example.com/filler:1", 18)
			c.list(fill.objects())
			for i := range 18 {
				c.list(fill.pod(i, names[i%3]))
			}
			c.after(5 * time.Minute)
			w := c.deployment("shop", "web", "registry.example.com/web:7", 3)
			w.setReady(0)
			c.create(w.objects())
			message := "0/3 nodes are available: 3 Too many pods. " +
				"preemption: 0/3 nodes are available: 3 No preemption " +
				"victims found for incoming pod."
			for i := range 3 {
				c.create(w.pod(i, "", pendingUnscheduled(message)))
			}
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				for i := range 3 {
					pod := last(c, w.pod(i, ""))
					c.warn(c.warningEvent(pod, "Pod", "FailedScheduling",
						message, "default-scheduler", m))
				}
			}
		},
	}
}
