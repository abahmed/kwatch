package scenarios

import "time"

// webhookIdleBackendGone: an operator was uninstalled and left its
// fail-closed validating webhook behind. The webhook matches only the
// operator's own resources, so for an hour nothing is created that it
// would reject. One notification says what will happen; no page.
func webhookIdleBackendGone() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-idle-backend-gone",
			Description: "A fail-closed webhook outlives its uninstalled " +
				"operator and no create is refused for an hour: one " +
				"notification, no page.",
			Root:        "validatingwebhookconfiguration//widget-operator",
			Tier:        "notify",
			MaxMessages: 1,
			MustNotBlame: []string{"deployment/shop/orders",
				"node//n1"},
			Tail: duration(time.Hour),
		},
		build: func(c *cluster) { buildIdleWebhook(c, false) },
	}
}

// webhookIdleThenRejects: the same leftover webhook, until a create it
// matches is refused. Then it blocks creates for real and pages.
func webhookIdleThenRejects() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-idle-then-rejects",
			Description: "A leftover fail-closed webhook with no " +
				"backend is quiet for an hour; the first refused pod " +
				"create escalates it to a page.",
			Root:        "validatingwebhookconfiguration//widget-operator",
			Tier:        "page",
			MaxMessages: 3,
			MustNotBlame: []string{"deployment/shop/orders",
				"node//n1"},
		},
		build: func(c *cluster) { buildIdleWebhook(c, true) },
	}
}

// buildIdleWebhook uninstalls an operator that leaves its webhook behind,
// lets an hour pass, and, when refuse is set, has the API server refuse
// a pod create because the webhook cannot be called.
func buildIdleWebhook(c *cluster, refuse bool) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	operator := c.deployment("widget-system", "widget-operator",
		"registry.example.com/widget-operator:1.0", 1)
	c.list(operator.objects())
	pod := operator.pod(0, "n1")
	c.list(pod)
	service := clusterService(c, "widget-system", "widget-operator", 443)
	c.list(service)
	c.list(clusterSlice(c, "widget-system", "widget-operator", pod))
	c.list(clusterValidatingHook(c, "widget-operator",
		"validate.widget.example.com", "widget-system", "widget-operator"))
	orders := c.deployment("shop", "orders",
		"registry.example.com/orders:2", 2)
	c.list(orders.objects())
	c.list(orders.pod(0, "n1"), orders.pod(1, "n2"))
	c.after(time.Minute)

	// The uninstall removes everything but the webhook configuration.
	setReplicas(operator, 0)
	operator.setReady(0)
	c.update(operator.objects())
	c.remove(pod, service)
	c.update(clusterSlice(c, "widget-system", "widget-operator"))
	c.after(time.Hour)
	if refuse {
		refuseWidgetCreates(c, orders)
	}
}

// refuseWidgetCreates scales orders up; its ReplicaSet is told that the
// API server could not call the webhook.
func refuseWidgetCreates(c *cluster, orders *workload) {
	setReplicas(orders, 4)
	orders.setReady(2)
	c.update(orders.objects())
	message := "Internal error occurred: failed calling webhook " +
		"\"validate.widget.example.com\": failed to call webhook: Post " +
		"\"https://" + c.n("widget-operator") + "." +
		c.n("widget-system") + ".svc:443/validate?timeout=10s\": " +
		"service \"" + c.n("widget-operator") + "\" not found"
	admissionFailedCreates(c, []*workload{orders}, message, 4,
		45*time.Second)
}
