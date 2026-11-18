package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nodeRemovalTieScenarios are node removals next to failures they may or
// may not cause. A planned drain whose pods moved cleanly, removed in
// the same minute as an unrelated replacement whose sandbox never
// starts, is not a cause; a drain whose evicted pods find no room is.
func nodeRemovalTieScenarios() []scenario {
	return []scenario{
		drainBesideStuckReplacement(), drainEvictionsFindNoRoom(),
	}
}

// drainBesideStuckReplacement: node-replace-sandbox-stuck, while n2 of
// the same pool and zone is drained in a planned way: its web pod is
// evicted, starts on n4 within seconds, and n2 is removed twenty
// seconds before n3. Only the removal of n3 touches the stuck pods;
// the drained node ran none of them and must not be named.
func drainBesideStuckReplacement() scenario {
	return scenario{
		expect: expectation{
			Name: "drain-beside-stuck-replacement",
			Description: "A pool node is drained cleanly and removed in " +
				"the same minute as another node whose replacement " +
				"pods never set up their sandbox.",
			Root: "node//n3", Tier: "page", MaxMessages: 2,
			BootHeld: true, Tail: duration(5 * time.Minute),
			MustNotBlame: []string{"node//n2"},
		},
		build: func(c *cluster) {
			web := c.deployment("shop", "web",
				"registry.example.com/web:1", 2)
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n2"))
			buildSandboxReplace(c, 0, func(c *cluster) {
				cleanDrain(c, "n2", web, 1, "n4")
			})
		},
	}
}

// cleanDrain cordons a pool node, evicts replica r of w from it, starts
// its replacement on target and removes the node once it is empty.
func cleanDrain(c *cluster, node string, w *workload, r int,
	target string) {
	cordoned := last(c, workerNode(c, node))
	cordoned.Spec.Unschedulable = true
	cordoned.Spec.Taints = []corev1.Taint{{
		Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
	}}
	c.update(cordoned)
	evict(c, last(c, w.pod(r, node)))
	ready := *w.deployment.Spec.Replicas
	w.setReady(ready - 1)
	c.update(w.objects())
	c.create(w.pod(r+1, target, startedNow, notReady))
	c.after(20 * time.Second)
	c.update(w.pod(r+1, target, startedNow))
	w.setReady(ready)
	c.update(w.objects())
	c.remove(last(c, cordoned))
	c.after(20 * time.Second)
}

// evict deletes a pod through the eviction API, as a drain does.
func evict(c *cluster, live *corev1.Pod) {
	pod := live.DeepCopy()
	now := metav1.NewTime(c.now)
	pod.DeletionTimestamp = &now
	nodeDisruption(c, pod, "EvictionByEvictionAPI", "Eviction API: evicting")
	c.update(pod)
	c.remove(pod)
}

// drainEvictionsFindNoRoom: n3 is cordoned, its pods are evicted and it
// is removed; the replacements of its pods stay Pending for lack of
// cpu. The drained node is the cause, not the scheduler's verdict.
func drainEvictionsFindNoRoom() scenario {
	return scenario{
		expect: expectation{
			Name: "drain-evictions-find-no-room",
			Description: "A node is drained and removed; the " +
				"replacements of its evicted pods stay Pending for " +
				"lack of cpu.",
			Root: "node//n3", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"scheduling//Insufficient cpu",
				"deployment/shop/cart", "deployment/shop/search"},
		},
		build: buildDrainNoRoom,
	}
}

func buildDrainNoRoom(c *cluster) {
	n3 := c.node("n3", "zone-a")
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), n3)
	fleet := nodeFleet(c, "shop", []string{"cart", "search"},
		[]string{"n1", "n3"})
	c.after(time.Minute)
	n3.Spec.Unschedulable = true
	c.update(n3)
	for _, w := range fleet {
		evict(c, last(c, w.pod(1, "n3")))
		w.setReady(1)
		c.update(w.objects())
	}
	c.after(10 * time.Second)
	c.remove(last(c, n3))
	c.after(5 * time.Second)
	for _, w := range fleet {
		c.create(w.pod(2, "", pendingUnscheduled(insufficientCPU)))
		scheduleFailedEvents(c, w, insufficientCPU, 1)
	}
	c.after(4 * time.Minute)
}
