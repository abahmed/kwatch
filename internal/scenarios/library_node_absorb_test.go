package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// nodeAbsorbScenarios are node problems next to workloads that fail
// for their own reasons: the node must not take over their failures.
func nodeAbsorbScenarios() []scenario {
	return []scenario{crashLoopBeforeNodeDrain(),
		crashEverywhereNodePressure()}
}

// absorbStartLine is the line report stops with at every start.
const absorbStartLine = "Unhandled exception. " +
	"System.InvalidOperationException: persistence requires " +
	"connection string 'ReportDb'."

// crashLoopBeforeNodeDrain: report crash-loops on n1 at start, missing
// a connection string, for twenty minutes; then n1 is cordoned and
// removed in a planned drain. The drain explains nothing, and the crash
// loop stays report's own.
func crashLoopBeforeNodeDrain() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-loop-before-node-drain",
			Description: "A workload crash-loops at start for twenty " +
				"minutes; then the node it runs on is drained and " +
				"removed.",
			Root: "deployment/shop/report", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			w := c.deployment("shop", "report",
				"registry.example.com/report:3.1", 1)
			web := c.deployment("shop", "web", "registry.example.com/web:9",
				1)
			w.setReady(0)
			c.list(w.objects())
			c.list(web.objects())
			c.list(w.pod(0, "n1", crashLoop(1, "Error", absorbStartLine,
				3)), web.pod(0, "n1"))
			for restarts := int32(4); restarts <= 8; restarts++ {
				c.after(4 * time.Minute)
				c.update(w.pod(0, "n1", crashLoop(1, "Error",
					absorbStartLine, restarts)))
			}
			cordoned := last(c, n1)
			cordoned.Spec.Unschedulable = true
			cordoned.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
			}}
			c.update(cordoned)
			c.after(time.Minute)
			c.update(w.pod(0, "n1", crashLoop(1, "Error", absorbStartLine,
				9)))
			c.after(time.Minute)
			c.remove(last(c, n1))
			c.after(2 * time.Minute)
		},
	}
}

// crashEverywhereNodePressure: n1's tasks stall on memory; while the
// stall lasts, worker crash-loops on all three nodes on a message it
// cannot parse. The stall is n1's own digest line; the crash loop is
// worker's, on n1 as on the two healthy nodes.
func crashEverywhereNodePressure() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-everywhere-node-pressure",
			Description: "One node's tasks stall on memory while a " +
				"workload crash-loops on it and on two healthy nodes.",
			Root: "deployment/shop/worker", Tier: "notify", MaxMessages: 4,
			Unrelated: []string{"node//n1"},
		},
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "worker",
				"registry.example.com/worker:5.0", 3)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n3"))
			for minute := range 20 {
				nodePSI(c, "n1", 45)
				if minute >= 7 && minute%2 == 1 {
					w.setReady(0)
					c.update(w.deployment, w.replicaSet)
					for i, node := range nodes {
						c.update(w.pod(i, node, crashLoop(2, "Error",
							"Error: queue consumer: unexpected message "+
								"schema version 7", int32(minute-4))))
					}
				}
				c.after(time.Minute)
			}
		},
	}
}
