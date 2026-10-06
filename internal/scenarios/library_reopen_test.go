package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// reopenScenarios are failures that keep coming back or stay open for
// hours: the same incident must speak for them, not a new page each time.
func reopenScenarios() []scenario {
	return []scenario{
		pageFlapsReopens(), longPageReminder(), digestNeverPagedResolve(),
		rollupChronicWorsens(), unusualEventRecurs(),
	}
}

// nodeDownWithPod turns node n1 NotReady and its pod not ready, as a
// dying kubelet does.
func nodeDownWithPod(
	c *cluster, n1 *corev1.Node, w *workload, podAfter time.Duration,
) {
	setNodeReady(c, n1, false)
	c.after(podAfter)
	c.update(w.pod(0, "n1", notReady))
	w.setReady(1)
	c.update(w.deployment, w.replicaSet)
}

// nodeBackWithPod brings the node and its pod back.
func nodeBackWithPod(c *cluster, n1 *corev1.Node, w *workload) {
	setNodeReady(c, n1, true)
	c.update(w.pod(0, "n1"))
	w.setReady(2)
	c.update(w.deployment, w.replicaSet)
}

// pageFlapsReopens: a node that pages goes down and comes back, then
// fails again twice more within two hours. Only the first failure pages;
// each return is an update in the same thread under the same alert.
func pageFlapsReopens() scenario {
	return scenario{
		expect: expectation{
			Name: "page-flaps-reopens",
			Description: "A node that pages fails again twice more " +
				"within two hours of resolving; the first message " +
				"pages and the returns do not.",
			Root: "node//n1", Tier: "page", MaxMessages: 5,
			MustNotBlame: []string{"deployment/shop/api"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildPageFlaps(c, 2) },
	}
}

// buildPageFlaps fails node n1 once and then returns more times, each
// time after the previous failure resolved; the last failure lasts. The
// gaps are uneven, so no rhythm is recognised, and each outage lasts
// longer than the last.
func buildPageFlaps(c *cluster, returns int) {
	n1 := c.node("n1", "zone-a")
	c.list(n1, c.node("n2", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(time.Minute)
	for i := 0; i <= returns; i++ {
		nodeDownWithPod(c, n1, w, time.Duration(40+150*i)*time.Second)
		c.after(time.Duration(4+4*i) * time.Minute)
		if i < returns {
			nodeBackWithPod(c, n1, w)
			c.after(time.Duration(9+3*i) * time.Minute)
		}
	}
}

// longPageReminder: the node stays down for seven hours. The page is
// said once more at six hours and never in between.
func longPageReminder() scenario {
	return scenario{
		expect: expectation{
			Name:        "long-page-reminder",
			Description: "A node that pages stays down for seven hours.",
			Root:        "node//n1", Tier: "page", MaxMessages: 3,
			MustNotBlame: []string{"deployment/shop/api"},
			Tail:         duration(7*time.Hour + 10*time.Minute),
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(time.Minute)
			nodeDownWithPod(c, n1, w, 40*time.Second)
		},
	}
}

// digestNeverPagedResolve: an autoscaler that cannot find its target is
// a digest-tier incident. A digest names it, then it recovers; the next
// digest says so. No paging provider ever heard of it, so none may be
// told it resolved.
func digestNeverPagedResolve() scenario {
	return scenario{
		expect: expectation{
			Name: "digest-never-paged-resolve",
			Description: "A digest-tier autoscaler problem is listed in " +
				"a digest and then recovers.",
			Root: "horizontalpodautoscaler/shop/orders", Tier: "digest",
			MaxMessages: 3, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			message := "the HPA controller was unable to get the " +
				"target's current scale: deployments.apps \"orders\" " +
				"not found"
			hpa := clusterHPA(c, "shop", "orders", "False",
				"FailedGetScale", message)
			c.list(hpa)
			for n := int32(1); n <= 4; n++ {
				c.after(2 * time.Minute)
				c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
					"FailedGetScale", message,
					"horizontal-pod-autoscaler", n))
			}
			c.after(35 * time.Minute)
			orders := c.deployment("shop", "orders",
				"registry.example.com/orders:5", 2)
			c.create(orders.objects())
			c.update(clusterHPA(c, "shop", "orders", "True",
				"SucceededGetScale", "the HPA controller was able to "+
					"get the target's current scale"))
		},
	}
}

// rollupChronicWorsens: two crash loops start in the same minute, so one
// roll-up announces both. Later one of them grows from a single failing
// pod to six of eight: that is news for its own thread.
func rollupChronicWorsens() scenario {
	return scenario{
		expect: expectation{
			Name: "rollup-chronic-worsens",
			Description: "Two crash loops are announced in one roll-up; " +
				"one then grows from one failing pod to six.",
			Root: "deployment/shop/worker", Tier: "notify", MaxMessages: 6,
			OtherRoots: []string{"deployment/shop/api"},
			Tail:       duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			api := c.deployment("shop", "api",
				"registry.example.com/api:7", 8)
			worker := c.deployment("shop", "worker",
				"registry.example.com/worker:4", 2)
			c.list(api.objects())
			c.list(worker.objects())
			for i := range 8 {
				c.list(api.pod(i, "n1"))
			}
			c.list(worker.pod(0, "n2"), worker.pod(1, "n2"))
			c.after(time.Minute)
			crashing := func(w *workload, i int, restarts int32) {
				c.update(w.pod(i, "n1", startedNow, crashLoop(1, "Error",
					"panic: connection refused", restarts)))
			}
			crashing(api, 0, 4)
			crashing(worker, 0, 4)
			c.after(5 * time.Minute)
			crashing(api, 0, 8)
			crashing(worker, 0, 8)
			c.after(30 * time.Minute)
			for i := 1; i < 6; i++ {
				crashing(api, i, 4)
			}
		},
	}
}

// unusualEventRecurs: Kubernetes keeps reporting an event kwatch has no
// detector for on a Service. It stops long enough for the finding to
// clear, then comes back: it is reported again.
func unusualEventRecurs() scenario {
	return scenario{
		expect: expectation{
			Name: "unusual-event-recurs",
			Description: "A Service gets repeated FailedDeployModel " +
				"events, they stop, and they return.",
			Root: "service/shop/web", Tier: "digest", MaxMessages: 4,
			Tail: duration(40 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			svc := clusterService(c, "shop", "web", 80)
			c.list(svc)
			message := "Failed deploy model due to AccessDenied: not " +
				"authorized to perform elasticloadbalancing:" +
				"DescribeTargetGroups"
			sighting := func(count int32) {
				c.warn(c.warningEvent(svc, "Service", "FailedDeployModel",
					message, "service", count))
			}
			for n := int32(1); n <= 4; n++ {
				c.after(2 * time.Minute)
				sighting(n)
			}
			c.after(50 * time.Minute)
			for n := int32(5); n <= 14; n++ {
				sighting(n)
				c.after(2 * time.Minute)
			}
		},
	}
}
