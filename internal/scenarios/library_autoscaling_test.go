package scenarios

import (
	"fmt"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// autoscalingScenarios are autoscalers that reached their ceiling while
// the load keeps growing.
func autoscalingScenarios() []scenario {
	return []scenario{
		autoscalerCeilingCPU(), autoscalerCeilingRequests(), hpaAtMax(),
	}
}

// autoscalerCeilingCPU: a batch of orders drives the orders worker to
// 240% of its 60% CPU target. Its HPA scales from 4 to its maximum of
// 8 and reports ScalingLimited (TooManyReplicas). The saturated pods
// time out their readiness probes. The HPA's ceiling is the root: the
// pods and nodes are fine, there are just too few replicas allowed.
func autoscalerCeilingCPU() scenario {
	return scenario{
		expect: expectation{
			Name: "autoscaler-ceiling-cpu",
			Description: "An HPA on CPU reaches maxReplicas under load " +
				"and reports ScalingLimited; saturated replicas time out " +
				"their readiness probes.",
			Root: "horizontalpodautoscaler/orders/worker", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//w7", "node//w8",
				"deployment/orders/worker"},
		},
		build: func(c *cluster) {
			ceiling := autoscalerCeiling{
				namespace: "orders", name: "worker", from: 4, max: 8,
				target: 60, nodes: []string{"w7", "w8"},
				probe: "Readiness probe failed: Get \"http://10.244.3.7:" +
					"9000/ready\": context deadline exceeded",
			}
			ceiling.build(c, []int32{130, 240})
		},
	}
}

// autoscalerCeilingRequests: the search API's HPA scales on CPU too,
// from 3 to its maximum of 5, and stays limited while utilization sits
// at 150% of its 80% target. Requests queue up and readiness checks
// time out with a client timeout. The HPA is the root.
func autoscalerCeilingRequests() scenario {
	return scenario{
		expect: expectation{
			Name: "autoscaler-ceiling-requests",
			Description: "An HPA reaches maxReplicas and stays limited; " +
				"the busy replicas fail readiness with client timeouts.",
			Root: "horizontalpodautoscaler/search/api", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//w1", "deployment/search/api"},
		},
		build: func(c *cluster) {
			ceiling := autoscalerCeiling{
				namespace: "search", name: "api", from: 3, max: 5,
				target: 80, nodes: []string{"w1"},
				probe: "Readiness probe failed: Get \"http://10.244.1.4:" +
					"8080/healthz\": net/http: request canceled " +
					"(Client.Timeout exceeded while awaiting headers)",
			}
			ceiling.build(c, []int32{120, 150})
		},
	}
}

// autoscalerCeiling describes one Deployment and its CPU HPA.
type autoscalerCeiling struct {
	namespace, name string
	from, max       int32
	target          int32
	nodes           []string
	probe           string
}

// build runs the Deployment at from replicas, scales it to max while
// utilization climbs through cpu, then holds it at the ceiling while
// the original replicas fail readiness.
func (a autoscalerCeiling) build(c *cluster, cpu []int32) {
	for _, node := range a.nodes {
		c.list(c.node(node, "zone-a"))
	}
	w := c.deployment(a.namespace, a.name,
		"registry.example.com/"+a.name+":3", a.from)
	c.list(w.objects())
	for i := range int(a.from) {
		c.list(w.pod(i, a.node(i)))
	}
	c.list(a.hpa(c, a.from, a.from, a.target-10, false))
	c.after(3 * time.Minute)
	setReplicas(w, a.max)
	w.setReady(a.from)
	c.update(w.objects())
	c.update(a.hpa(c, a.from, a.max, cpu[0], false))
	for i := int(a.from); i < int(a.max); i++ {
		c.create(w.pod(i, a.node(i), startedNow))
	}
	w.setReady(a.max)
	c.update(w.objects())
	c.after(90 * time.Second)
	c.update(a.hpa(c, a.max, a.max, cpu[1], true))
	for round := int32(1); round <= 5; round++ {
		for i := range int(a.from) {
			pod := w.pod(i, a.node(i), notReady)
			c.update(pod)
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", a.probe,
				"kubelet", round*2))
		}
		w.setReady(a.max - a.from)
		c.update(w.objects())
		c.after(time.Minute)
	}
}

func (a autoscalerCeiling) node(i int) string {
	return a.nodes[i%len(a.nodes)]
}

// hpa is the autoscaler with the observed CPU utilization; limited sets
// ScalingLimited to TooManyReplicas.
func (a autoscalerCeiling) hpa(c *cluster, current, desired, cpu int32,
	limited bool) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := clusterHPA(c, a.namespace, a.name, "True", "ValidMetricFound",
		"the HPA was able to successfully calculate a replica count "+
			"from cpu resource utilization (percentage of request)")
	minReplicas := a.from
	target := a.target
	hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas = &minReplicas, a.max
	hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name: corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: &target,
			},
		},
	}}
	hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas = current,
		desired
	hpa.Status.CurrentMetrics = []autoscalingv2.MetricStatus{{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricStatus{
			Name: corev1.ResourceCPU,
			Current: autoscalingv2.MetricValueStatus{
				AverageUtilization: &cpu,
			},
		},
	}}
	status, reason := corev1.ConditionFalse, "DesiredWithinRange"
	message := "the desired count is within the acceptable range"
	if limited {
		status, reason = corev1.ConditionTrue, "TooManyReplicas"
		message = fmt.Sprintf("the desired replica count is more than "+
			"the maximum replica count of %d", a.max)
	}
	hpa.Status.Conditions = append(hpa.Status.Conditions,
		autoscalingv2.HorizontalPodAutoscalerCondition{
			Type: autoscalingv2.ScalingLimited, Status: status,
			Reason: reason, Message: message,
			LastTransitionTime: metav1.NewTime(c.now),
		})
	return hpa
}

// hpaAtMax: a traffic spike drives the web Deployment's CPU to
// 185% of its target. The HPA scales to its maximum of 6 and reports
// ScalingLimited (TooManyReplicas); the replicas are saturated and
// start failing readiness on timeouts. The HPA's ceiling is the root:
// the pods and nodes are healthy, there are just not enough of them.
// Held out until 2026-10-01 as
// heldout-hpa-at-max; labelled since its miss was looked at
// (SCORECARD.md, held-out rotation).
func hpaAtMax() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-at-max",
			Description: "An HPA reaches maxReplicas under load and " +
				"reports ScalingLimited; the saturated replicas fail " +
				"readiness on timeouts.",
			Root: "horizontalpodautoscaler/shop/web", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"deployment/shop/web"},
		},
		build: buildHPAAtMax,
	}
}

func buildHPAAtMax(c *cluster) {
	nodes := []string{"n1", "n2"}
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:9", 3)
	c.list(w.objects())
	for i := range 3 {
		c.list(w.pod(i, nodes[i%2]))
	}
	c.list(hpaAtMaxAutoscaler(c, 3, 3, 55, false))
	c.after(2 * time.Minute)
	setReplicas(w, 6)
	w.setReady(3)
	c.update(w.objects())
	c.update(hpaAtMaxAutoscaler(c, 3, 6, 140, false))
	for i := 3; i < 6; i++ {
		c.create(w.pod(i, nodes[i%2], startedNow))
	}
	w.setReady(6)
	c.update(w.objects())
	c.after(time.Minute)
	c.update(hpaAtMaxAutoscaler(c, 6, 6, 185, true))
	probe := "Readiness probe failed: Get \"http://10.244.0.12:8080/" +
		"healthz\": context deadline exceeded (Client.Timeout exceeded " +
		"while awaiting headers)"
	for round := int32(1); round <= 4; round++ {
		for i := range 3 {
			pod := w.pod(i, nodes[i%2], notReady)
			c.update(pod)
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", probe,
				"kubelet", round*3))
		}
		w.setReady(3)
		c.update(w.objects())
		c.after(time.Minute)
	}
}

// hpaAtMaxAutoscaler is the web HPA (2 to 6 replicas, 70% CPU target)
// with the observed CPU utilization; limited sets ScalingLimited.
func hpaAtMaxAutoscaler(c *cluster, current, desired, cpu int32,
	limited bool) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := clusterHPA(c, "shop", "web", "True", "ValidMetricFound",
		"the HPA was able to successfully calculate a replica count "+
			"from cpu resource utilization (percentage of request)")
	target := int32(70)
	hpa.Spec.MaxReplicas = 6
	hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name: corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: &target,
			},
		},
	}}
	hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas = current,
		desired
	hpa.Status.CurrentMetrics = []autoscalingv2.MetricStatus{{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricStatus{
			Name: corev1.ResourceCPU,
			Current: autoscalingv2.MetricValueStatus{
				AverageUtilization: &cpu,
			},
		},
	}}
	status, reason, message := corev1.ConditionFalse, "DesiredWithinRange",
		"the desired count is within the acceptable range"
	if limited {
		status, reason = corev1.ConditionTrue, "TooManyReplicas"
		message = "the desired replica count is more than the maximum " +
			"replica count"
	}
	hpa.Status.Conditions = append(hpa.Status.Conditions,
		autoscalingv2.HorizontalPodAutoscalerCondition{
			Type: autoscalingv2.ScalingLimited, Status: status,
			Reason: reason, Message: message,
			LastTransitionTime: metav1.NewTime(c.now),
		})
	return hpa
}
