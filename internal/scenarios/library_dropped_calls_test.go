package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// droppedCallScenarios are releases that stop naming a Service the
// previous revision called: the pods of the new revision no longer
// declare it, which only the workload's template shows.
func droppedCallScenarios() []scenario {
	return []scenario{
		{expect: expectation{
			Name: "rollout-drops-service-dependency",
			Description: "A release removes the environment value that " +
				"names the payments Service from the API's pod " +
				"template, and the new pods crash.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1",
				"deployment/shop/payments", "service/shop/payments"},
		}, build: func(c *cluster) { buildDropsPaymentsCall(c, false) }},
		{expect: expectation{
			Name: "rollout-drops-service-dependency-and-back",
			Description: "A release removes the environment value that " +
				"names the payments Service and the next one, 20 " +
				"seconds later, puts it back; nothing fails.",
			Quiet: true,
		}, build: func(c *cluster) { buildDropsPaymentsCall(c, true) }},
	}
}

// withPaymentsCall sets or clears the API's PAYMENTS_ADDR.
func withPaymentsCall(c *cluster, call bool) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = nil
		if call {
			spec.Containers[0].Env = []corev1.EnvVar{{
				Name: "PAYMENTS_ADDR", Value: c.n("payments") + ":8080"}}
		}
	}
}

// buildDropsPaymentsCall lists the API calling the payments Service,
// then rolls out a release without that call. With back the next
// release restores it 20 seconds later and every pod is ready;
// otherwise the new pods crash with a missing setting.
func buildDropsPaymentsCall(c *cluster, back bool) {
	c.list(c.node("n1", "zone-a"))
	payments := c.deployment("shop", "payments",
		"registry.example.com/payments:3", 1)
	c.list(payments.objects())
	paymentsPod := payments.pod(0, "n1")
	c.list(paymentsPod, clusterService(c, "shop", "payments", 8080),
		trafficSlice(c, "payments", paymentsPod))
	api := c.deployment("shop", "api", "registry.example.com/api:5", 2)
	configTemplate(api, withPaymentsCall(c, true))
	c.list(api.objects())
	c.list(api.pod(0, "n1"), api.pod(1, "n1"))
	c.after(10 * time.Minute)
	rs := api.rollout(withPaymentsCall(c, false))
	c.update(api.deployment)
	c.create(rs)
	c.after(20 * time.Second)
	if back {
		api.rollout(withPaymentsCall(c, true))
		api.setReady(2)
		c.update(api.objects())
		c.after(10 * time.Minute)
		return
	}
	c.create(api.pod(0, "n1", startedNow, notReady))
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(api.pod(0, "n1", startedNow, crashLoop(1, "Error",
			"panic: missing setting PAYMENTS_ADDR", restarts)))
		c.after(45 * time.Second)
	}
}
