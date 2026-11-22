package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap4bNodeScenarios are incidents of several nodes: one change that
// breaks the old nodes together, a workload that takes down each node it
// reaches, an agent that does the same, and failures that heal alone.
func gap4bNodeScenarios() []scenario {
	return []scenario{
		gap4bOldNodesLoseNetwork(), gap4bKubeletStatusLag(),
		gap4bHogMovesNodes(), gap4bAllReplicasOneNode(),
		gap4bAgentSystemOOM(), gap4bNodesBlipTogether(),
		gap4bDataplaneSaturated(),
	}
}

// gap4bNetworkMessage is what a kubelet says when its CNI configuration
// is gone.
const gap4bNetworkMessage = "container runtime network not ready: " +
	"NetworkReady=false reason:NetworkPluginNotReady message:Network " +
	"plugin returns error: cni plugin not initialized"

// gap4bOldNodesLoseNetwork: an OS update runs on every running node in
// the same hour and removes the routes the CNI installed. Four old nodes
// go NotReady two minutes apart; a node that joined ten minutes ago, after
// the update, stays healthy.
func gap4bOldNodesLoseNetwork() scenario {
	return scenario{
		expect: g2Exp("old-nodes-lose-network-together",
			"Four nodes that were running during an OS update lose their "+
				"pod networking within eight minutes of each other while "+
				"a node that joined after it stays Ready; pods on the "+
				"four go unready.",
			"node//n1", "page", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart",
			"node//n5").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3", "n4", "n5"}
			for _, name := range nodes {
				node := c.node(name, "zone-a")
				if name == "n5" {
					node.CreationTimestamp = metav1.NewTime(
						c.start.Add(-10 * time.Minute))
				}
				c.list(node)
			}
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, nodes)
			c.after(5 * time.Minute)
			for i, name := range nodes[:4] {
				g2NodeCondition(c, name, corev1.NodeReady,
					corev1.ConditionFalse, "KubeletNotReady",
					gap4bNetworkMessage)
				c.after(20 * time.Second)
				for _, w := range fleet {
					c.update(w.pod(i, name, notReady))
					w.setReady(int32(4 - i))
					c.update(w.objects())
				}
				c.after(100 * time.Second)
			}
			c.after(5 * time.Minute)
		},
	}
}

// gap4bKubeletStatusLag: a node that starts and stops many short-lived
// pods hits the kubelet's client rate limit; the status of its pods lags
// by minutes, so the pods read Pending although their containers run.
func gap4bKubeletStatusLag() scenario {
	return scenario{
		expect: g2Exp("node-kubelet-status-updates-lag",
			"Pods on a node that churns many short-lived pods stay "+
				"Pending with no status for minutes because the "+
				"kubelet's status updates are rate limited.",
			"node//n1", "notify", "deployment/ci/runner").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("ci", "runner", "registry.example.com/r:3", 8)
			w.setReady(0)
			c.create(w.objects())
			var pods []*corev1.Pod
			for i := range 8 {
				pod := w.pod(i, "n1", gap2NoScheduler)
				pods = append(pods, pod)
				c.create(pod)
			}
			for range 4 {
				c.after(2 * time.Minute)
				for _, pod := range pods {
					c.update(pod.DeepCopy())
				}
			}
		},
	}
}

// gap4bHogMovesNodes: a batch workload without limits grows until its
// node runs out of memory and stops reporting; the Deployment starts it
// on the next node, which fails the same way.
func gap4bHogMovesNodes() scenario {
	return scenario{
		expect: g2Exp("memory-hog-moves-and-takes-down-nodes",
			"One workload without memory limits is placed on the next "+
				"node after each node it ran on reports a system OOM "+
				"and stops reporting.",
			"deployment/batch/ingest", "page", "node//n1", "node//n2",
			"node//n3", "deployment/shop/orders", "deployment/shop/payments",
			"deployment/shop/cart").budget(4).tail(10 * time.Minute),
		build: func(c *cluster) {
			names := []string{"n1", "n2", "n3"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, names)
			hog := c.deployment("batch", "ingest",
				"registry.example.com/ingest:9", 1)
			c.list(hog.objects())
			c.after(2 * time.Minute)
			for i, name := range names {
				c.create(hog.pod(i, name, startedNow))
				c.after(2 * time.Minute)
				node := g2NodeCondition(c, name, corev1.NodeMemoryPressure,
					corev1.ConditionTrue, "KubeletHasInsufficientMemory",
					"kubelet has insufficient memory available")
				g2Event(c, node, "Node", "SystemOOM", "System OOM "+
					"encountered, victim process: ingest, pid: 4121",
					"kubelet", 1)
				nodeLose(c, node)
				c.after(30 * time.Second)
				for _, w := range fleet {
					c.update(w.pod(i, name, notReady))
					w.setReady(2)
					c.update(w.objects())
				}
				c.after(time.Minute)
				c.remove(hog.pod(i, name))
			}
			c.after(5 * time.Minute)
		},
	}
}

// gap4bAllReplicasOneNode: nothing spreads the three replicas of web or
// its cache; all four pods run on n1, which stops reporting.
func gap4bAllReplicasOneNode() scenario {
	return scenario{
		expect: g2Exp("all-replicas-on-one-node-lost",
			"All replicas of a service and its cache run on one node, "+
				"which stops reporting; the service has no ready pod.",
			"node//n1", "page", "deployment/shop/web",
			"deployment/shop/cache").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			web := c.deployment("shop", "web", "registry.example.com/web:7", 3)
			cache := c.deployment("shop", "cache",
				"registry.example.com/cache:4", 1)
			c.list(web.objects())
			c.list(cache.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n1"), web.pod(2, "n1"),
				cache.pod(0, "n1"))
			c.after(10 * time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			for _, w := range []*workload{web, cache} {
				for i := range int(*w.deployment.Spec.Replicas) {
					c.update(w.pod(i, "n1", notReady))
				}
				w.setReady(0)
				c.update(w.objects())
			}
			c.after(6 * time.Minute)
		},
	}
}

// gap4bAgentSystemOOM: the log shipper runs without a memory limit and
// grows on every node; each node reports a system OOM that names it, goes
// NotReady and comes back, twice.
func gap4bAgentSystemOOM() scenario {
	return scenario{
		expect: g2Exp("daemonset-without-limits-system-oom-cycle",
			"A node agent without a memory limit grows on every node; "+
				"each node reports a system OOM naming it and flaps "+
				"NotReady and Ready, twice.",
			"daemonset/observability/logship", "notify", "node//n1",
			"node//n2", "node//n3").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			ds := g2Daemon(c, "observability", "logship", nodes...)
			c.after(5 * time.Minute)
			for round := int32(1); round <= 2; round++ {
				for _, name := range nodes {
					node := g2NodeCondition(c, name, corev1.NodeReady,
						corev1.ConditionFalse, "KubeletNotReady",
						"container runtime is down")
					g2Event(c, node, "Node", "SystemOOM", "System OOM "+
						"encountered, victim process: logship, pid: 3302",
						"kubelet", round)
					c.update(agentPod(c, ds, name, crashLoop(137,
						"OOMKilled", "", round)))
					c.after(90 * time.Second)
					g2NodeCondition(c, name, corev1.NodeReady,
						corev1.ConditionTrue, "KubeletReady",
						"kubelet is posting ready status")
					c.after(time.Minute)
				}
			}
		},
	}
}

// gap4bNodesBlipTogether: the API server's address changes; every node
// misses its status update for three minutes and reports Unknown, then
// all return. No pod changes state.
func gap4bNodesBlipTogether() scenario {
	return scenario{
		expect: g2Quiet("api-endpoint-change-nodes-blip-together-quiet",
			"Every node misses its status updates for three minutes "+
				"after the API server's address changes and then "+
				"reports Ready again; pods keep running."),
		build: func(c *cluster) {
			n1, _ := gapNodeFleet(c)
			c.after(5 * time.Minute)
			nodes := []*corev1.Node{n1, c.node("n2", "zone-a"),
				c.node("n3", "zone-a")}
			for _, node := range nodes {
				nodeLose(c, node)
			}
			c.after(3 * time.Minute)
			for _, node := range nodes {
				g2NodeCondition(c, node.Name[:2], corev1.NodeReady,
					corev1.ConditionTrue, "KubeletReady",
					"kubelet is posting ready status")
			}
			c.after(10 * time.Minute)
		},
	}
}

// gap4bDataplaneSaturated: one pod opens and closes connections without
// end; the CNI agent on its node is saturated, fails its probes and
// restarts, and pods of three namespaces on that node time out.
func gap4bDataplaneSaturated() scenario {
	return scenario{
		expect: g2Exp("node-cni-agent-saturated-by-connections",
			"The CNI agent of one node is saturated by a pod's connection "+
				"churn and fails its probes; pods of three namespaces on "+
				"that node time out while other nodes are fine.",
			"daemonset/kube-system/cni-agent", "notify", "node//n2",
			"deployment/shop/web", "deployment/billing/invoices",
			"deployment/media/transcode", "cluster-dns//cluster-dns").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			ds := g2Daemon(c, "kube-system", "cni-agent", nodes...)
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, nodes)
			c.after(5 * time.Minute)
			lookup := "dial tcp 10.96.14.7:5432: i/o timeout"
			for round := int32(1); round <= 6; round++ {
				setAgentCounts(ds, 3, 2)
				pod := agentPod(c, ds, "n2", notReady)
				c.update(ds, pod)
				gapKubelet(c, pod, "Unhealthy", "Liveness probe failed: "+
					"Get \"http://127.0.0.1:9099/liveness\": context "+
					"deadline exceeded (Client.Timeout exceeded while "+
					"awaiting headers)", round)
				if round >= 2 {
					gap2CrashPods(c, fleet, nodes, []int{1}, lookup,
						round-1)
				}
				c.after(time.Minute)
			}
		},
	}
}
