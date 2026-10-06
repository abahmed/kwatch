package scenarios

import (
	"time"
)

// metricsBlipScenarios are short and long losses of the metrics API
// seen by many autoscalers at once.
func metricsBlipScenarios() []scenario {
	return []scenario{
		metricsBlip(2*time.Minute, "metrics-blip-short"),
		metricsBlip(20*time.Minute, "metrics-blip-long"),
	}
}

// metricsBlip: ten HPAs in ten namespaces cannot read metrics for
// length. A blip of two minutes is metrics-server restarting and is not
// news. Twenty minutes is an outage, reported once for all ten HPAs.
func metricsBlip(length time.Duration, name string) scenario {
	expect := expectation{
		Name: name, Quiet: true, Tail: duration(30 * time.Minute),
		Description: "Ten HPAs cannot read metrics for " +
			length.String() + " while metrics-server runs.",
	}
	if length >= 10*time.Minute {
		expect.Quiet = false
		expect.Root = "apiservice//v1beta1.metrics.k8s.io"
		expect.Tier = "digest"
		expect.MaxMessages = 3
	}
	return scenario{expect: expect, build: func(c *cluster) {
		buildMetricsBlip(c, length)
	}}
}

func buildMetricsBlip(c *cluster, length time.Duration) {
	c.list(c.node("n1", "zone-a"))
	server := metricsServer(c)
	c.list(server.objects())
	c.list(server.pod(0, "n1"), server.pod(1, "n1"))
	c.list(clusterService(c, "kube-system", "metrics-server", 10250))
	c.list(metricsSlice(c, server, true))
	c.list(clusterAPIService(c, "True", "Passed", "all checks passed"))
	namespaces := []string{"shop", "billing", "search", "orders", "users",
		"media", "ledger", "mail", "auth", "jobs"}
	for _, ns := range namespaces {
		w := c.deployment(ns, "web", "registry.example.com/web:7", 2)
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n1"))
		c.list(clusterHPA(c, ns, "web", "True", "ValidMetricFound", ""))
	}
	c.after(time.Minute)
	for _, ns := range namespaces {
		hpa := clusterHPA(c, ns, "web", "False",
			"FailedGetResourceMetric", metricsServerFailure)
		c.update(hpa)
		c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
			"FailedGetResourceMetric", metricsServerFailure,
			"horizontal-pod-autoscaler", 1))
	}
	for n := int32(2); time.Duration(n)*30*time.Second <= length; n++ {
		c.after(30 * time.Second)
		for _, ns := range namespaces {
			hpa := clusterHPA(c, ns, "web", "False",
				"FailedGetResourceMetric", metricsServerFailure)
			c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
				"FailedGetResourceMetric", metricsServerFailure,
				"horizontal-pod-autoscaler", n))
		}
	}
	for _, ns := range namespaces {
		c.update(clusterHPA(c, ns, "web", "True", "ValidMetricFound", ""))
	}
}
