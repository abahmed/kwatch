package scenarios

import (
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// scalingWaveScenarios are a cluster that removes nodes in numbers on a
// schedule. What the scaling does by itself (a routed workload's only
// pod moving to a node that has room) must not page while the nodes
// come and go; a failure that outlives the scaling, or one the scaling
// does not explain (a crash loop, a failed image pull, an outage on a
// node the scaling never touched), still pages. Each is studied alone:
// a wave is judged against the size of the cluster, which the staging
// day's mix would change.
func scalingWaveScenarios() []scenario {
	return []scenario{
		scaleDownMovedPodHeld(), scaleDownMovedPodNeverStarts(),
		scaleDownCrashLoopPages(), scaleDownUnrelatedOutageTold(),
		scaleDownMovedPodPullFails(),
	}
}

// waveNoRoom is the scheduler's answer while the remaining nodes are
// full.
const waveNoRoom = "0/2 nodes are available: 2 Insufficient cpu."

// waveFleet lists five nodes and the routed shop web Deployment, its
// only pod on n1, then lets half an hour pass.
func waveFleet(c *cluster) (*workload, []*corev1.Node) {
	nodes := make([]*corev1.Node, 0, 5)
	for i := range 5 {
		nodes = append(nodes, c.node("n"+strconv.Itoa(i+1), "zone-a"))
	}
	for _, n := range nodes {
		c.list(n)
	}
	web := c.deployment("shop", "web", "registry.example.com/web:4", 1)
	c.list(web.objects())
	pod := web.pod(0, "n1")
	c.list(pod, clusterService(c, "shop", "web", 8080),
		trafficSlice(c, "web", pod), trafficIngress(c, "web", ""))
	c.after(30 * time.Minute)
	return web, nodes
}

// movedPod is web's replacement pod on node, created at born: the
// ReplicaSet makes it when the old pod goes, not an hour before the
// story as the other pods of the fixture.
func movedPod(
	web *workload, node string, born time.Time, states ...podState,
) *corev1.Pod {
	pod := web.pod(1, node, states...)
	pod.CreationTimestamp = metav1.NewTime(born)
	return pod
}

// waveScaleDown removes n1, n2 and n3 one minute apart, as a nightly
// scale-down does. web's pod goes with n1 and its replacement waits
// for room. It returns when the replacement was created.
func waveScaleDown(
	c *cluster, web *workload, nodes []*corev1.Node,
) time.Time {
	born := c.now
	c.remove(nodes[0])
	c.remove(web.pod(0, "n1"))
	web.setReady(0)
	c.update(web.objects())
	c.update(trafficSlice(c, "web"))
	c.create(movedPod(web, "", born, pendingUnscheduled(waveNoRoom)))
	for _, n := range nodes[1:3] {
		c.after(time.Minute)
		c.remove(n)
	}
	return born
}

// waveStartsOn schedules web's replacement on n5 and lets it pull its
// image for three minutes, then become ready.
func waveStartsOn(c *cluster, web *workload, born time.Time) {
	pod := movedPod(web, "n5", born)
	creating(c, pod)
	c.update(pod)
	c.after(3 * time.Minute)
	ready := movedPod(web, "n5", born, startedNow)
	c.update(ready)
	web.setReady(1)
	c.update(web.objects())
	c.update(trafficSlice(c, "web", ready))
}

// scaleDownMovedPodHeld: three of five nodes are removed within two
// minutes. The routed web's only pod went with the first; its
// replacement waits four minutes for room, pulls its image and is ready
// nine minutes after the move. Nobody is paged for the scaling.
func scaleDownMovedPodHeld() scenario {
	return scenario{
		expect: expectation{
			Name: "scale-down-moved-pod-held",
			Description: "Three of five nodes are removed within two " +
				"minutes; the only pod of a routed Deployment moves, " +
				"waits for room, pulls its image and is ready nine " +
				"minutes later.",
			Quiet: true, Alone: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			web, nodes := waveFleet(c)
			born := waveScaleDown(c, web, nodes)
			c.after(4 * time.Minute)
			waveStartsOn(c, web, born)
		},
	}
}

// scaleDownMovedPodNeverStarts: the same scale-down, but the moved pod
// never finds room. Once the nodes have settled it is a real outage and
// pages once, blaming the removed node whose capacity is missing.
func scaleDownMovedPodNeverStarts() scenario {
	return scenario{
		expect: expectation{
			Name: "scale-down-moved-pod-never-starts",
			Description: "Three of five nodes are removed within two " +
				"minutes; the only pod of a routed Deployment moves " +
				"and its replacement never finds room.",
			// Any removed node took capacity: n1 and n2 are a fair
			// hedge.
			Root: "node//n1", Tier: "page", Ambiguous: true,
			// Held on purpose until the nodes settle, as a boot is.
			BootHeld: true, Alone: true,
			MaxMessages: 2, Tail: duration(30 * time.Minute),
			MustNotBlame: []string{"deployment/shop/web"},
		},
		build: func(c *cluster) {
			web, nodes := waveFleet(c)
			waveScaleDown(c, web, nodes)
			c.after(20 * time.Minute)
		},
	}
}

// scaleDownCrashLoopPages: during the same scale-down the moved pod
// starts at once and crash-loops on its own error. A crash loop is
// never a scaling side effect: it pages without waiting for the nodes.
func scaleDownCrashLoopPages() scenario {
	return scenario{
		expect: expectation{
			Name: "scale-down-crash-loop-pages",
			Description: "Three of five nodes are removed within two " +
				"minutes; the moved pod of a routed Deployment starts " +
				"on a remaining node and crash-loops.",
			Root: "deployment/shop/web", Tier: "page", MaxMessages: 2,
			Alone: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			web, nodes := waveFleet(c)
			born := waveScaleDown(c, web, nodes)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(movedPod(web, "n5", born, crashLoop(1, "Error",
					"panic: invalid configuration value", restarts)))
				c.after(time.Minute)
			}
		},
	}
}
