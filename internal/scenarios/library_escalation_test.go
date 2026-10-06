package scenarios

import (
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
)

// escalationScenarios are failures that change after they were
// announced: they get worse, last longer than routine allows, or come
// back, and the thread has to say so once without repeating itself.
func escalationScenarios() []scenario {
	return []scenario{
		crashloopAfterAnnounce(), notifyFlapReopens(), causeFlipStable(),
		knownBootStillDemotes(), knownCrashloopPastBoot(),
		hpaResolveWhileWorkloadBroken(),
	}
}

// causeFlipStable: the pods behind a Service crash and recover five times
// in half an hour, the Service losing its endpoints each time, as the
// reasoning blames a pod, then the Service, then the Deployment for the
// same failures. It is one incident and no round after the first is
// news: the cause's name flipping is not a material change.
func causeFlipStable() scenario {
	return scenario{
		expect: expectation{
			Name: "cause-flip-stable",
			Description: "The pods behind a Service crash and recover " +
				"five times in half an hour, the Service losing its " +
				"endpoints each time; no round re-posts.",
			Root: "deployment/shop/draftor", Tier: "notify",
			MaxMessages:  3,
			MustNotBlame: []string{"service/shop/draftor", "node//n1"},
			Tail:         duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "draftor",
				"registry.example.com/draftor:9", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "draftor", 8080),
				trafficSlice(c, "draftor", pods...))
			c.after(time.Minute)
			for round := int32(1); round <= 5; round++ {
				crashBehind(c, w, pods, "draftor",
					"panic: draftor: template cache is corrupt", round)
				c.update(w.pod(0, "n1"), w.pod(1, "n1"))
				w.setReady(2)
				c.update(w.objects())
				c.update(trafficSlice(c, "draftor", pods...))
				c.after(time.Minute)
			}
		},
	}
}

// crashBehind crashes both pods of w for four minutes with the slice of
// service unready.
func crashBehind(
	c *cluster, w *workload, pods []*corev1.Pod, service, message string,
	round int32,
) {
	for step := int32(0); step < 4; step++ {
		restarts := round*4 + step
		c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
			restarts)), w.pod(1, "n1", crashLoop(1, "Error",
			message, restarts)))
		w.setReady(0)
		c.update(w.objects())
		c.update(trafficUnreadySlice(c, service, pods...))
		c.after(time.Minute)
	}
}

// crashloopAfterAnnounce: a Deployment's pod stops passing its readiness
// probe and is announced as not ready. Minutes later the container
// starts to crash-loop. That is one update, and the crash loop going on
// for another quarter of an hour is not another.
func crashloopAfterAnnounce() scenario {
	return scenario{
		expect: expectation{
			Name: "crashloop-after-announce",
			Description: "A pod that was announced as not ready begins " +
				"to crash-loop; the incident says so once.",
			Root: "deployment/shop/pay", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "pay",
				"registry.example.com/pay:5", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(time.Minute)
			c.update(w.pod(0, "n1", notReady))
			w.setReady(1)
			c.update(w.objects())
			c.after(8 * time.Minute)
			for restarts := int32(4); restarts <= 9; restarts += 5 {
				c.update(w.pod(0, "n1", crashLoop(1, "Error",
					"panic: pay: ledger schema is older than the code",
					restarts)))
				c.after(7 * time.Minute)
			}
		},
	}
}

// notifyFlapReopens: a Deployment crash-loops, is fixed, and fails again
// within two hours. The first message announces it; the return is an
// update of the same incident, "failing again", not a new incident. It
// recovers once more and fails a third time before its longer hold ends,
// which adds no message.
func notifyFlapReopens() scenario {
	return scenario{
		expect: expectation{
			Name: "notify-flap-reopens",
			Description: "A crash-looping Deployment recovers and fails " +
				"again twice within two hours.",
			Root: "deployment/shop/mailer", Tier: "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "mailer",
				"registry.example.com/mailer:2", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(time.Minute)
			message := "panic: mailer: smtp relay rejected the credentials"
			for i, healthy := range []time.Duration{
				5 * time.Minute, 8 * time.Minute, 0,
			} {
				crashFor(c, w, message,
					10*time.Minute+time.Duration(i)*time.Minute)
				if healthy > 0 {
					recoverFor(c, w, healthy)
				}
			}
		},
	}
}

// crashFor puts the workload's only pod into a crash loop for d.
func crashFor(c *cluster, w *workload, message string, d time.Duration) {
	c.update(w.pod(0, "n1", crashLoop(1, "Error", message, 6)))
	w.setReady(0)
	c.update(w.objects())
	c.after(d)
}

// recoverFor makes the pod ready again for d.
func recoverFor(c *cluster, w *workload, d time.Duration) {
	c.update(w.pod(0, "n1", startedNow))
	w.setReady(1)
	c.update(w.objects())
	c.after(d)
}

// knownBootStillDemotes: a workload that crashed on each of two earlier
// days, and was heard about both times, is known. On the third day it
// fails for six minutes, inside the boot window: routine boot noise, so
// the digest carries it and nobody is interrupted.
func knownBootStillDemotes() scenario {
	return scenario{
		expect: expectation{
			Name: "known-boot-still-demotes",
			Description: "A known crash loop at the start of the day " +
				"lasts six minutes, inside the boot window.",
			Root: "deployment/shop/sync", Tier: "notify", MaxMessages: 5,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(time.Hour),
		},
		build: func(c *cluster) {
			buildKnownCrash(c, "panic: sync: cursor table is locked",
				6*time.Minute)
		},
	}
}

// knownCrashloopPastBoot: the same known workload, but on the third day
// it crash-loops for thirty minutes with nothing ready. Past the boot
// window that is not routine, so the incident is announced again.
func knownCrashloopPastBoot() scenario {
	return scenario{
		expect: expectation{
			Name: "known-crashloop-past-boot",
			Description: "A known crash loop at the start of the day " +
				"lasts thirty minutes with no ready replica.",
			Root: "deployment/shop/sync", Tier: "notify", MaxMessages: 7,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(time.Hour),
		},
		build: func(c *cluster) {
			buildKnownCrash(c, "panic: sync: batch window is closed",
				30*time.Minute)
		},
	}
}

// buildKnownCrash runs the sync Deployment through two days with a
// twelve-minute crash loop each, then a third day's crash of length
// last.
func buildKnownCrash(c *cluster, message string, last time.Duration) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "sync", "registry.example.com/sync:7", 1)
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	c.after(time.Minute)
	for day, length := range []time.Duration{
		12 * time.Minute, 12 * time.Minute, last,
	} {
		crashFor(c, w, message, length)
		recoverFor(c, w, 23*time.Hour+time.Duration(day)*time.Minute)
	}
}

// hpaResolveWhileWorkloadBroken: an autoscaler at its maximum clears
// while the Deployment it scales is down: both pods crash-loop on the
// same bad dependency. The autoscaler's incident does not resolve
// ("stable", "healthy") while the workload behind it still fails.
func hpaResolveWhileWorkloadBroken() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-resolve-while-workload-broken",
			Description: "An HPA at its maximum stops being limited " +
				"while the Deployment it scales crash-loops with no " +
				"ready replica.",
			Root: "deployment/shop/warehouse", Tier: "notify",
			OtherRoots:   []string{"horizontalpodautoscaler/shop/warehouse"},
			MaxMessages:  4,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "warehouse",
				"registry.example.com/warehouse:8", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.list(warehouseHPA(c, 2, 2, 400, true))
			c.after(4 * time.Minute)
			for i := range 2 {
				c.update(w.pod(i, "n1", crashLoop(1, "Error",
					"panic: warehouse: stock service answered 503", 6)))
			}
			w.setReady(0)
			c.update(w.objects())
			c.after(3 * time.Minute)
			c.update(warehouseHPA(c, 2, 2, 90, false))
			c.after(40 * time.Minute)
		},
	}
}

// warehouseHPA is the autoscaler of the warehouse Deployment: one to two
// replicas, limited or not. It is not pinned (min equal to max), which
// could never be maxed out.
func warehouseHPA(
	c *cluster, current, desired, cpu int32, limited bool,
) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := autoscalerAtMax(c, "shop", "warehouse", 2, current, desired,
		cpu, limited)
	floor := int32(1)
	hpa.Spec.MinReplicas = &floor
	return hpa
}
