package scenarios

import (
	"time"
)

// nodeClearedScenarios are node problems that end while the pods they
// explained still fail: the node stops explaining them.
func nodeClearedScenarios() []scenario {
	return []scenario{nodeStallClearsPodsStillUnready()}
}

// nodeStallClearsPodsStillUnready: n1's tasks stall on memory for ten
// minutes and the replicas of three workloads on it stop being ready,
// while their replicas on n2 are fine. The stall ends; the replicas on
// n1 stay unready and those on n2 turn unready too, for half an hour.
// The node explained them while it stalled; once its own finding is
// gone and the failures are no longer its own, its incident does not
// stay open on pods that fail for another reason.
func nodeStallClearsPodsStillUnready() scenario {
	return scenario{
		expect: expectation{
			Name: "node-stall-clears-pods-still-unready",
			Description: "A node stalls on memory and three workloads' " +
				"replicas on it turn unready; the stall ends but the " +
				"replicas stay unready.",
			Root: "node//n1", RevisesRoot: true,
			OtherRoots: []string{"deployment/shop/api",
				"deployment/shop/web", "deployment/shop/worker"},
			Tier: "notify", MaxMessages: 6,
		},
		build: buildNodeStallClears,
	}
}

func buildNodeStallClears(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	names := []string{"api", "web", "worker"}
	fleet := nodeFleet(c, "shop", names, []string{"n1", "n2"})
	c.after(2 * time.Minute)
	began := c.now
	for minute := range 40 {
		if minute < 10 {
			nodePSI(c, "n1", 60)
		} else {
			nodePSI(c, "n1", 0)
		}
		if minute >= 1 {
			for _, w := range fleet {
				if minute >= 12 {
					unreadyOn(c, w, 1, "n2", began.Add(12*time.Minute),
						int32(minute))
				}
				pod := w.pod(0, "n1", notReadySince(began.Add(
					time.Minute)))
				c.update(pod)
				ev := c.warningEvent(pod, "Pod", "Unhealthy",
					"Readiness probe failed: HTTP probe failed with "+
						"statuscode: 503", "kubelet", int32(minute))
				ev.InvolvedObject.FieldPath = "spec.containers{app}"
				c.warn(ev)
				w.setReady(1)
				c.update(w.deployment, w.replicaSet)
			}
		}
		c.after(time.Minute)
	}
}

// unreadyOn marks replica i of w on node unready since began, with
// count failed readiness probes.
func unreadyOn(c *cluster, w *workload, i int, node string,
	began time.Time, count int32) {
	pod := w.pod(i, node, notReadySince(began))
	c.update(pod)
	ev := c.warningEvent(pod, "Pod", "Unhealthy", "Readiness probe "+
		"failed: HTTP probe failed with statuscode: 503", "kubelet", count)
	ev.InvolvedObject.FieldPath = "spec.containers{app}"
	c.warn(ev)
}
