package scenarios

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// youngNodeScenarios are failures concentrated on a node that joined
// minutes ago, and the outage that must not be taken for one.
func youngNodeScenarios() []scenario {
	return []scenario{youngNodeTransient(), youngNodePersists(),
		youngAndOldNodesFail()}
}

// youngNodeLine is what the pods on the new node log: the cache is not
// reachable from there yet.
const youngNodeLine = "dial tcp 10.0.7.20:6379: connect: connection " +
	"refused"

var youngNodeApps = []string{"orders", "payments", "search"}

// youngNodeTransient: node n3 joins; the new replicas of three services
// on it cannot reach the cache for two minutes, then recover. Their
// replicas on n1, an old node, stay healthy. A start-up transient of
// the new node: nothing is said.
func youngNodeTransient() scenario {
	return scenario{
		expect: expectation{
			Name: "young-node-transient",
			Description: "Pods on a node that joined a minute ago cannot " +
				"reach the cache for two minutes while their replicas " +
				"on an old node stay healthy; then they recover.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) { buildYoungNode(c, 2, false) },
	}
}

// youngNodePersists: the same, but the pods on n3 still fail when n3
// is no longer new. They are reported then, with the node's age. The
// young-node hold ends kube.YoungNodeWindow after n3 joined, but the
// message waits for the Deployments' "1 of 2 replicas are ready", which
// the pool's boot grace (kube.BootWindow) holds: hence BootHeld.
func youngNodePersists() scenario {
	return scenario{
		expect: expectation{
			Name: "young-node-persists",
			Description: "Pods on a node that joined a minute ago cannot " +
				"reach the cache and still cannot when the node is no " +
				"longer new: reported once the node's window is over.",
			Root: "failure-signature//dial tcp <ip>:6379: connect: " +
				"connection refused",
			Tier: "notify", MaxMessages: 2, BootHeld: true,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) { buildYoungNode(c, 16, false) },
	}
}

// youngAndOldNodesFail: the cache is down for everyone; replicas on the
// old node fail with the new node's. A real outage: one incident
// named by the cache, told as soon as it settles.
func youngAndOldNodesFail() scenario {
	return scenario{
		expect: expectation{
			Name: "young-and-old-nodes-fail",
			Description: "The cache is down: replicas on an old node and " +
				"on a node that joined a minute ago fail together.",
			Root: "external-endpoint//10.0.7.20:6379", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n3"},
		},
		build: func(c *cluster) { buildYoungNode(c, 6, true) },
	}
}

// buildYoungNode runs three services with a replica on old node n1; n3
// joins and each scales out to a second replica there, which fails for
// minutes minutes. With oldToo the replicas on n1 fail as well.
func buildYoungNode(c *cluster, minutes int, oldToo bool) {
	c.list(c.node("n1", "zone-a"))
	var fleet []*workload
	for _, app := range youngNodeApps {
		w := c.deployment("shop", app, "registry.example.com/"+app+":2", 1)
		w.setReady(1)
		c.list(w.objects())
		c.list(w.pod(0, "n1"))
		fleet = append(fleet, w)
	}
	c.after(5 * time.Minute)
	n3 := c.node("n3", "zone-a")
	n3.CreationTimestamp = metav1.NewTime(c.now)
	c.list(n3)
	for _, w := range fleet {
		// Scaled out: the new replica lands on the new node.
		two := int32(2)
		w.deployment.Spec.Replicas, w.replicaSet.Spec.Replicas = &two, &two
		c.update(w.pod(1, "n3"))
		w.setReady(1)
		c.update(w.deployment, w.replicaSet)
	}
	c.after(time.Minute)
	for restarts := int32(1); restarts <= int32(minutes); restarts++ {
		for _, w := range fleet {
			c.update(w.pod(1, "n3", crashLoop(1, "Error", youngNodeLine,
				restarts)))
			if oldToo {
				c.update(w.pod(0, "n1", crashLoop(1, "Error",
					youngNodeLine, restarts)))
				w.setReady(0)
				c.update(w.deployment, w.replicaSet)
			}
		}
		c.after(time.Minute)
	}
	for _, w := range fleet {
		c.update(w.pod(1, "n3"))
		c.update(w.pod(0, "n1"))
		w.setReady(2)
		c.update(w.deployment, w.replicaSet)
	}
	c.after(10 * time.Minute)
}
