package scenarios

import (
	"time"
)

// readyNeverScenarios are workloads and autoscalers that look healthy to
// every crash and restart check but never do their job.
func readyNeverScenarios() []scenario {
	return []scenario{readyNever(), hpaTargetMissing()}
}

// readyNever: a controller's pod runs, never restarts and never passes
// its readiness probe. Nothing crashes, so only the workload can say it.
func readyNever() scenario {
	return scenario{
		expect: expectation{
			Name: "ready-never",
			Description: "A Deployment's only pod runs for two hours " +
				"without restarting and its readiness probe never " +
				"passes.",
			Root: "deployment/shop/cert-controller", Tier: "notify",
			MaxMessages: 3, Tail: duration(30 * time.Minute),
		},
		build: buildReadyNever,
	}
}

func buildReadyNever(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "cert-controller",
		"registry.example.com/cert-controller:1.0", 1)
	w.setReady(0)
	c.list(w.objects())
	c.list(w.pod(0, "n1", startedNow, notReady))
	probe := "Readiness probe failed: Get \"http://10.0.1.4:8080/readyz\": " +
		"context deadline exceeded"
	pod := w.pod(0, "n1", startedNow, notReady)
	for count := int32(3); count <= 9; count += 3 {
		c.after(5 * time.Minute)
		c.warn(c.warningEvent(pod, "Pod", "Unhealthy", probe, "kubelet",
			count))
	}
}

// hpaTargetMissing: an autoscaler names a Deployment that was deleted.
// It is configuration clutter, not an outage: the digest says it once.
func hpaTargetMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-target-missing",
			Description: "An HPA keeps reporting that the Deployment it " +
				"scales does not exist.",
			Root:        "horizontalpodautoscaler/istio-system/istiod",
			Tier:        "digest",
			MaxMessages: 2, Tail: duration(30 * time.Minute),
		},
		build: buildHPATargetMissing,
	}
}

func buildHPATargetMissing(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	message := "the HPA controller was unable to get the target's " +
		"current scale: deployments.apps \"istiod\" not found"
	hpa := clusterHPA(c, "istio-system", "istiod", "False",
		"FailedGetScale", message)
	c.list(hpa)
	for n := int32(1); n <= 4; n++ {
		c.after(2 * time.Minute)
		c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
			"FailedGetScale", message, "horizontal-pod-autoscaler", n))
	}
}
