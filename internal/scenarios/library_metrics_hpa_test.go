package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// metricsHPAScenarios are autoscalers that cannot read metrics: all of
// them because the metrics API is down, or one of them for its own
// reasons.
func metricsHPAScenarios() []scenario {
	return []scenario{metricsAPIHPAs(), oneHPABadTarget()}
}

// metricsServerFailure is the text of an HPA that cannot reach the
// metrics API.
const metricsServerFailure = "the HPA was unable to compute the " +
	"replica count: failed to get cpu utilization: unable to get " +
	"metrics for resource cpu: unable to fetch metrics from resource " +
	"metrics API: the server is currently unable to handle the " +
	"request (get pods.metrics.k8s.io)"

// metricsServer is the metrics-server Deployment, serving on the port
// its Service targets.
func metricsServer(c *cluster) *workload {
	w := c.deployment("kube-system", "metrics-server",
		"registry.k8s.io/metrics-server/metrics-server:v0.7.1", 2)
	port := []corev1.ContainerPort{{Name: "https", ContainerPort: 10250}}
	w.deployment.Spec.Template.Spec.Containers[0].Ports = port
	w.replicaSet.Spec.Template.Spec.Containers[0].Ports = port
	return w
}

// metricsSlice is the Service's EndpointSlice. While every pod fails
// the endpoints are still listed, none of them ready.
func metricsSlice(
	c *cluster, w *workload, ready bool,
) *discoveryv1.EndpointSlice {
	slice := clusterSlice(c, "kube-system", "metrics-server",
		w.pod(0, "n1"), w.pod(1, "n1"))
	port := int32(10250)
	slice.Ports = []discoveryv1.EndpointPort{{Port: &port}}
	for i := range slice.Endpoints {
		slice.Endpoints[i].Conditions.Ready = boolPtr(ready)
	}
	return slice
}

// metricsAPIHPAs: the metrics-server Deployment crash-loops while the
// APIService still reports Available. Four HPAs in four namespaces fail
// to read CPU metrics. One incident is expected, rooted at the
// metrics-server Deployment, not four autoscaler incidents.
func metricsAPIHPAs() scenario {
	return scenario{
		expect: expectation{
			Name: "metrics-api-hpas",
			Description: "metrics-server crash-loops while its " +
				"APIService still reads Available; four HPAs cannot " +
				"read CPU metrics.",
			Root: "deployment/kube-system/metrics-server",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"horizontalpodautoscaler/shop/web"},
		},
		build: buildMetricsAPIHPAs,
	}
}

func buildMetricsAPIHPAs(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	server := metricsServer(c)
	c.list(server.objects())
	c.list(server.pod(0, "n1"), server.pod(1, "n1"))
	c.list(clusterService(c, "kube-system", "metrics-server", 10250))
	c.list(metricsSlice(c, server, true))
	c.list(clusterAPIService(c, "True", "Passed", "all checks passed"))
	namespaces := []string{"shop", "billing", "search", "orders"}
	for _, ns := range namespaces {
		w := c.deployment(ns, "web", "registry.example.com/web:7", 2)
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n1"))
		c.list(clusterHPA(c, ns, "web", "True", "ValidMetricFound", ""))
	}
	c.after(time.Minute)
	crash := "Error: tls: failed to load serving certificate"
	for _, restarts := range []int32{3, 4, 5, 6, 7, 8} {
		c.update(server.pod(0, "n1", crashLoop(1, "Error", crash, restarts)),
			server.pod(1, "n1", crashLoop(1, "Error", crash, restarts)))
		c.update(metricsSlice(c, server, false))
		for _, ns := range namespaces {
			hpa := clusterHPA(c, ns, "web", "False",
				"FailedGetResourceMetric", metricsServerFailure)
			c.update(hpa)
			c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
				"FailedGetResourceMetric", metricsServerFailure,
				"horizontal-pod-autoscaler", restarts))
		}
		c.after(30 * time.Second)
	}
}

// oneHPABadTarget: one HPA names a custom metric no adapter serves
// while the metrics API and the other autoscalers are fine. The HPA is
// its own incident; the metrics API is not blamed.
func oneHPABadTarget() scenario {
	return scenario{
		expect: expectation{
			Name: "one-hpa-bad-target",
			Description: "One HPA reads a custom metric nobody serves; " +
				"the metrics API and the other HPAs are healthy.",
			Root: "horizontalpodautoscaler/shop/web", Tier: "digest",
			MaxMessages: 2,
			MustNotBlame: []string{"apiservice//v1beta1.metrics.k8s.io",
				"deployment/kube-system/metrics-server"},
		},
		build: buildOneHPABadTarget,
	}
}

func buildOneHPABadTarget(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	server := metricsServer(c)
	c.list(server.objects())
	c.list(server.pod(0, "n1"), server.pod(1, "n1"))
	c.list(clusterService(c, "kube-system", "metrics-server", 10250))
	c.list(metricsSlice(c, server, true))
	c.list(clusterAPIService(c, "True", "Passed", "all checks passed"))
	namespaces := []string{"shop", "billing", "search", "orders"}
	for _, ns := range namespaces {
		w := c.deployment(ns, "web", "registry.example.com/web:7", 2)
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n1"))
		c.list(clusterHPA(c, ns, "web", "True", "ValidMetricFound", ""))
	}
	c.after(time.Minute)
	message := "the HPA was unable to compute the replica count: " +
		"failed to get pods metric value: unable to get metric " +
		"requests_per_second: no matching metrics found for " +
		"requests_per_second"
	for n := int32(1); n <= 6; n++ {
		hpa := clusterHPA(c, "shop", "web", "False",
			"FailedGetPodsMetric", message)
		c.update(hpa)
		c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
			"FailedGetPodsMetric", message,
			"horizontal-pod-autoscaler", n))
		c.after(30 * time.Second)
	}
}
