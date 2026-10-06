package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// flipScenarios are failures whose blamed object keeps changing between
// members of one workload chain, or that keep coming back.
func flipScenarios() []scenario {
	return []scenario{serviceBackendFlip(), webhookFlapsHourly()}
}

// serviceBackendFlip: the pods of cart crash and recover twice; each
// time the Service loses its ready endpoints and gets them back. The
// Deployment stays the root throughout: the Service, a pod and the
// Deployment are one chain, so re-blaming them is not a revised cause.
func serviceBackendFlip() scenario {
	return scenario{
		expect: expectation{
			Name: "service-backend-flip",
			Description: "The pods behind a Service crash and recover " +
				"twice, the Service losing its endpoints each time; " +
				"the story is one incident with no revised cause.",
			Root: "deployment/shop/cart", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"service/shop/cart", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "cart",
				"registry.example.com/cart:6", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "cart", 8080),
				trafficSlice(c, "cart", pods...))
			c.after(time.Minute)
			for round := int32(1); round <= 2; round++ {
				flipCrash(c, w, pods, round)
				c.update(w.pod(0, "n1"), w.pod(1, "n1"))
				w.setReady(2)
				c.update(w.objects())
				c.update(trafficSlice(c, "cart", pods...))
				c.after(90 * time.Second)
			}
			c.after(10 * time.Minute)
		},
	}
}

// flipCrash crashes both pods of w for four minutes, with the Service
// slice unready.
func flipCrash(c *cluster, w *workload, pods []*corev1.Pod, round int32) {
	for step := int32(0); step < 4; step++ {
		restarts := round*4 + step
		c.update(w.pod(0, "n1", crashLoop(1, "Error",
			"panic: cart cache is full", restarts)),
			w.pod(1, "n1", crashLoop(1, "Error",
				"panic: cart cache is full", restarts)))
		w.setReady(0)
		c.update(w.objects())
		c.update(trafficUnreadySlice(c, "cart", pods...))
		c.after(time.Minute)
	}
}

// webhookFlapsHourly: the image-policy webhook times out for a couple of
// minutes, recovers, and does it again every quarter of an hour. Each
// recurrence is the same chronic problem; the incident stays one thread
// instead of resolving and re-opening each time.
func webhookFlapsHourly() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-flaps-hourly",
			Description: "A validating webhook times out for two " +
				"minutes every quarter of an hour; the incident does " +
				"not ping-pong between healthy and failing.",
			Root: "validatingwebhookconfiguration//image-policy",
			Tier: "page", MaxMessages: 3,
			MustNotBlame: []string{"deployment/payments/gateway",
				"node//n1", "node//n2"},
		},
		build: buildWebhookFlaps,
	}
}

func buildWebhookFlaps(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	hook := c.deployment("policy-system", "image-policy",
		"registry.example.com/image-policy:1.4", 2)
	c.list(hook.objects())
	pods := []*corev1.Pod{hook.pod(0, "n1"), hook.pod(1, "n2")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "policy-system", "image-policy", 443))
	c.list(admissionSlice(c, "policy-system", "image-policy", 443,
		pods...))
	config := clusterValidatingHook(c, "image-policy",
		"image-policy.example.com", "policy-system", "image-policy")
	timeout := int32(10)
	config.Webhooks[0].TimeoutSeconds = &timeout
	c.list(config)
	app := c.deployment("payments", "gateway",
		"registry.example.com/gw:8", 2)
	c.list(app.objects())
	c.list(app.pod(0, "n1"), app.pod(1, "n2"))
	message := "Internal error occurred: failed calling webhook " +
		"\"image-policy.example.com\": failed to call webhook: Post " +
		"\"https://" + c.n("image-policy") + "." + c.n("policy-system") +
		".svc:443/validate?timeout=10s\": context deadline exceeded"
	c.after(2 * time.Minute)
	setReplicas(app, 3)
	app.setReady(2)
	c.update(app.objects())
	for episode := 0; episode < 5; episode++ {
		admissionFailedCreates(c, []*workload{app}, message, 3,
			45*time.Second)
		c.after(13 * time.Minute)
	}
}
