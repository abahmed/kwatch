package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// nodePressureEvictedLinger: n1 runs low on memory and evicts one pod
// of each of twelve workloads in four namespaces. The pressure clears
// and the Failed pods stay listed, as they do until the cluster
// collects them. They are history of the pressure incident, not new
// failures: their replacements run.
func nodePressureEvictedLinger() scenario {
	return scenario{
		expect: expectation{
			Alone: true,
			Name:  "node-pressure-evicted-linger",
			Description: "Memory pressure on a node evicts pods of twelve " +
				"workloads; the pressure clears and the evicted pods " +
				"stay listed, and nothing new is announced for them.",
			Root: "node//n1", Tier: "notify", MaxMessages: 2,
			Tail:         duration(40 * time.Minute),
			MustNotBlame: []string{"zone//zone-a"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			var fleet []*workload
			for _, ns := range groupNamespaces {
				var names []string
				for i := range 3 {
					names = append(names, fmt.Sprintf("%s-app%d", ns, i))
				}
				fleet = append(fleet,
					nodeFleet(c, ns, names, []string{"n1", "n2"})...)
			}
			c.after(time.Minute)
			c.update(lifecycleMemoryPressure(c, n1))
			c.after(20 * time.Second)
			for _, w := range fleet {
				pod := w.pod(0, "n1", nodeEvictedForMemory)
				c.update(pod)
				c.warn(c.warningEvent(pod, "Pod", "Evicted",
					pod.Status.Message, "kubelet", 1))
				w.setReady(1)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(20 * time.Second)
			for _, w := range fleet {
				c.create(w.pod(2, "n3", startedNow))
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(8 * time.Minute)
			relieved := last(c, n1)
			setNodeCondition(relieved, corev1.NodeMemoryPressure,
				corev1.ConditionFalse, "KubeletHasSufficientMemory",
				"kubelet has sufficient memory available", c.now)
			relieved.Spec.Taints = nil
			c.update(relieved)
		},
	}
}

// nodeLateCauseAfterCrashes: four workloads on n1 crash-loop with
// different errors while the node still reports Ready. Four minutes
// later n1 goes NotReady. The node is the cause the workloads share:
// they join its incident, and its recovery ends the outage.
func nodeLateCauseAfterCrashes() scenario {
	return scenario{
		expect: expectation{
			Alone: true,
			Name:  "node-late-cause-after-crashes",
			Description: "Workloads on one node crash first and the node " +
				"is reported NotReady four minutes later: the workloads " +
				"join the node incident and it recovers as one.",
			Root: "node//n1", Tier: "page", MaxMessages: 4, MaxPages: 1,
			// The workloads are named until the node reports NotReady.
			RevisesRoot: true,
			Tail:        duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			var fleet []*workload
			for _, ns := range groupNamespaces {
				fleet = append(fleet,
					nodeFleet(c, ns, []string{ns + "-a"},
						[]string{"n1", "n2"})...)
			}
			c.after(time.Minute)
			errs := []string{
				"panic: cannot allocate memory",
				"OCI runtime exec failed: read init-p: connection reset",
				"fatal: disk I/O error", "Error: ETIMEDOUT",
			}
			for r := int32(1); r <= 5; r++ {
				for i, w := range fleet {
					c.update(w.pod(0, "n1", crashLoop(1, "Error", errs[i], r)))
					w.setReady(1)
					c.update(w.deployment, w.replicaSet)
				}
				c.after(time.Minute)
			}
			nodeLose(c, n1)
			c.after(10 * time.Minute)
			nodeRecover(c, n1)
			c.after(30 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1"))
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
			}
		},
	}
}
