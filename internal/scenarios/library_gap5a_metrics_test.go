package scenarios

import "time"

// gap5aMetricsScenarios are resource metrics that cannot be scraped from
// kubelets while the metrics API itself is available.
func gap5aMetricsScenarios() []scenario {
	return []scenario{gap5aMetricsX509(), gap5aServingCSRPending()}
}

// gap5aMetricsStory plays an HPA that reports message while the metrics
// APIService is Available; extra runs before the failures start.
func gap5aMetricsStory(c *cluster, message string, extra func(*cluster)) {
	c.list(c.node("n1", "zone-a"))
	server := metricsServer(c)
	c.list(server.objects())
	c.list(server.pod(0, "n1"), server.pod(1, "n1"))
	c.list(clusterService(c, "kube-system", "metrics-server", 10250))
	c.list(metricsSlice(c, server, true))
	c.list(clusterAPIService(c, "True", "Passed", "all checks passed"))
	w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"))
	c.list(clusterHPA(c, "shop", "web", "True", "ValidMetricFound", ""))
	c.after(time.Minute)
	if extra != nil {
		extra(c)
	}
	for m := int32(1); m <= 6; m++ {
		hpa := clusterHPA(c, "shop", "web", "False",
			"FailedGetResourceMetric", message)
		c.update(hpa)
		g2Event(c, hpa, "HorizontalPodAutoscaler", "FailedGetResourceMetric",
			message, "horizontal-pod-autoscaler", m)
		c.after(time.Minute)
	}
}

// gap5aMetricsX509: metrics-server cannot verify the serving certificate
// of a kubelet; the APIService is Available, HPAs get no data.
func gap5aMetricsX509() scenario {
	return scenario{
		expect: g2Exp("metrics-server-cannot-verify-kubelet-certificate",
			"metrics-server fails to scrape a kubelet because of a "+
				"certificate name mismatch; the metrics APIService is "+
				"Available and the HPA reports FailedGetResourceMetric.",
			"unknown", "digest",
			"apiservice//v1beta1.metrics.k8s.io", "deployment/shop/web",
			"node//n1").tail(20 * time.Minute),
		build: func(c *cluster) {
			gap5aMetricsStory(c, "failed to get cpu utilization: unable "+
				"to get metrics for resource cpu: unable to fetch metrics "+
				"from resource metrics API: unable to fully scrape "+
				"metrics from node n1: x509: certificate is valid for "+
				"10.0.0.5, not 10.0.0.9", nil)
		},
	}
}

// gap5aServingCSRPending: a node's kubelet serving certificate request
// was never approved; the kubelet serves a self-signed certificate that
// metrics-server rejects.
func gap5aServingCSRPending() scenario {
	return scenario{
		expect: g2Exp("kubelet-serving-csr-pending-metrics-fail",
			"A kubelet serving certificate request stays Pending; the "+
				"node's self-signed certificate is rejected by "+
				"metrics-server and the HPA reports "+
				"FailedGetResourceMetric.",
			"certificatesigningrequest//csr-4k8x2", "digest",
			"deployment/kube-system/metrics-server", "node//n1",
			"deployment/shop/web").tail(20 * time.Minute),
		build: func(c *cluster) {
			gap5aMetricsStory(c, "failed to get cpu utilization: unable "+
				"to get metrics for resource cpu: unable to fetch metrics "+
				"from resource metrics API: unable to fully scrape "+
				"metrics from node n1: x509: certificate signed by "+
				"unknown authority", func(c *cluster) {
				c.create(builtinObject(c, "certificates.k8s.io/v1",
					"CertificateSigningRequest", "", "csr-4k8x2",
					map[string]any{"signerName": "kubernetes.io/kubelet-" +
						"serving"}, map[string]any{}))
			})
		},
	}
}
