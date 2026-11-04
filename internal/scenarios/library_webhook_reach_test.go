package scenarios

import (
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
)

// webhookReachScenarios are fail-closed webhooks whose backend is gone,
// told apart by whether their rules intercept objects that exist now.
func webhookReachScenarios() []scenario {
	return []scenario{webhookClosedOnPods(), webhookClosedOnGoneCRDs(),
		webhookIgnoreOnPods()}
}

// withRules makes the configuration's webhook intercept creates and
// updates of resource in group.
func withRules(
	config *admissionv1.ValidatingWebhookConfiguration,
	group, resource string,
) *admissionv1.ValidatingWebhookConfiguration {
	config.Webhooks[0].Rules = []admissionv1.RuleWithOperations{{
		Operations: []admissionv1.OperationType{
			admissionv1.Create, admissionv1.Update},
		Rule: admissionv1.Rule{APIGroups: []string{group},
			APIVersions: []string{"*"}, Resources: []string{resource}},
	}}
	return config
}

// guardedShop lists two nodes and a running two-replica Deployment.
func guardedShop(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	orders := c.deployment("shop", "orders", "registry.example.com/o:2", 2)
	c.list(orders.objects())
	c.list(orders.pod(0, "n1"), orders.pod(1, "n2"))
}

// webhookClosedOnPods: a validating webhook that fails closed covers
// pod creates and updates, and the Service it calls is gone. Pods exist,
// so their creates and updates are rejected now: one notification.
func webhookClosedOnPods() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-closed-on-pods-backend-gone",
			Description: "A fail-closed webhook covering pods calls a " +
				"Service that does not exist while pods exist: one " +
				"notification that pod writes are rejected.",
			Root:        "validatingwebhookconfiguration//guard-pods",
			Tier:        "notify",
			MaxMessages: 2, Tail: duration(time.Hour),
			MustNotBlame: []string{"deployment/shop/orders", "node//n1"},
		},
		build: func(c *cluster) {
			guardedShop(c)
			c.list(withRules(clusterValidatingHook(c, "guard-pods",
				"validate.pods.guard.example.com", "guard-system",
				"guard-svc"), "", "pods"))
			c.after(time.Minute)
		},
	}
}

// webhookClosedOnGoneCRDs: a leftover webhook fails closed and calls a
// Service that is gone, but its rules cover only custom resources whose
// definition was removed. Nothing it covers exists, so no write is
// rejected: a digest line, no notification.
func webhookClosedOnGoneCRDs() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-closed-on-gone-crds-backend-gone",
			Description: "A fail-closed webhook whose Service is gone " +
				"covers only custom resources that no longer exist: a " +
				"digest line, no notification.",
			Root: "validatingwebhookconfiguration//widget-guard",
			Tier: "digest", MaxMessages: 2, Tail: duration(time.Hour),
			MustNotBlame: []string{"deployment/shop/orders"},
		},
		build: func(c *cluster) {
			guardedShop(c)
			c.list(withRules(clusterValidatingHook(c, "widget-guard",
				"validate.widgets.example.com", "widget-system",
				"widget-svc"), "widgets.example.com", "widgets"))
			c.after(time.Minute)
		},
	}
}

// webhookIgnoreOnPods: the same webhook covering pods, but it ignores
// failure, so requests skip it and nothing is rejected: a digest line.
func webhookIgnoreOnPods() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-ignore-on-pods-backend-gone",
			Description: "A webhook that ignores failure covers pods and " +
				"calls a Service that does not exist: a digest line, no " +
				"notification.",
			Root: "validatingwebhookconfiguration//guard-pods",
			Tier: "digest", MaxMessages: 2, Tail: duration(time.Hour),
			MustNotBlame: []string{"deployment/shop/orders"},
		},
		build: func(c *cluster) {
			guardedShop(c)
			c.list(ignoringHook(withRules(clusterValidatingHook(c,
				"guard-pods", "validate.pods.guard.example.com",
				"guard-system", "guard-svc"), "", "pods")))
			c.after(time.Minute)
		},
	}
}
