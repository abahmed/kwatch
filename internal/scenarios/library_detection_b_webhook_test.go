package scenarios

import (
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
)

// detectionBWebhookScenarios are admission webhooks whose backend Service
// is gone or has nothing ready, told apart by their failure policy.
func detectionBWebhookScenarios() []scenario {
	return []scenario{
		webhookNamespaceGone(), webhookIgnoreNoEndpoints(),
		webhookIgnoreBesideClosed(),
	}
}

// ignoringHook makes a validating webhook configuration skip calls it
// cannot make.
func ignoringHook(
	config *admissionv1.ValidatingWebhookConfiguration,
) *admissionv1.ValidatingWebhookConfiguration {
	ignore := admissionv1.Ignore
	config.Webhooks[0].FailurePolicy = &ignore
	return config
}

// webhookNamespaceGone: a policy engine was uninstalled with its whole
// namespace, and its webhook configurations stayed. Four fail closed and
// call a Service in a namespace that does not exist; one ignores failure.
// Nothing was refused yet, so it is one notification, no page.
func webhookNamespaceGone() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-backend-namespace-gone",
			Description: "Fail-closed webhooks call Services in a " +
				"namespace that no longer exists, and one ignores " +
				"failure: one notification, no page.",
			Root: "service/policy-gone/policy-svc",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"node//n1"},
			Tail: duration(time.Hour),
		},
		build: buildWebhookNamespaceGone,
	}
}

func buildWebhookNamespaceGone(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	orders := c.deployment("shop", "orders", "registry.example.com/o:2", 2)
	c.list(orders.objects())
	c.list(orders.pod(0, "n1"), orders.pod(1, "n2"))
	c.list(clusterValidatingHook(c, "policy-resource",
		"validate.resource.policy.example.com", "policy-gone",
		"policy-svc"))
	c.list(clusterValidatingHook(c, "policy-exception",
		"validate.exception.policy.example.com", "policy-gone",
		"policy-svc"))
	c.list(clusterValidatingHook(c, "policy-cleanup",
		"validate.cleanup.policy.example.com", "policy-gone",
		"policy-svc"))
	c.list(clusterValidatingHook(c, "policy-global",
		"validate.global.policy.example.com", "policy-gone",
		"policy-svc"))
	c.list(ignoringHook(clusterValidatingHook(c, "policy-ttl",
		"validate.ttl.policy.example.com", "policy-gone",
		"policy-svc")))
	c.after(time.Minute)
}

// webhookIgnoreNoEndpoints: an operator was scaled to zero and its
// webhook ignores failure, so calls skip it and nothing is blocked. It is
// a digest line, never a notification.
func webhookIgnoreNoEndpoints() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-ignore-no-endpoints",
			Description: "A webhook that ignores failure has no ready " +
				"endpoints behind it: a digest line, not a " +
				"notification.",
			Root:        "validatingwebhookconfiguration//widget-operator",
			Tier:        "digest",
			MaxMessages: 2, Tail: duration(time.Hour),
			MustNotBlame: []string{"deployment/shop/orders"},
		},
		build: buildWebhookIgnoreNoEndpoints,
	}
}

func buildWebhookIgnoreNoEndpoints(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	operator := c.deployment("widget-system", "widget-operator",
		"registry.example.com/widget-operator:1.0", 1)
	c.list(operator.objects())
	pod := operator.pod(0, "n1")
	c.list(pod)
	c.list(clusterService(c, "widget-system", "widget-operator", 443))
	c.list(clusterSlice(c, "widget-system", "widget-operator", pod))
	c.list(ignoringHook(clusterValidatingHook(c, "widget-operator",
		"validate.widget.example.com", "widget-system",
		"widget-operator")))
	orders := c.deployment("shop", "orders", "registry.example.com/o:2", 2)
	c.list(orders.objects())
	c.list(orders.pod(0, "n1"), orders.pod(1, "n2"))
	c.after(time.Minute)

	setReplicas(operator, 0)
	operator.setReady(0)
	c.update(operator.objects())
	c.remove(pod)
	c.update(clusterSlice(c, "widget-system", "widget-operator"))
	c.after(30 * time.Minute)
}

// webhookIgnoreBesideClosed: one webhook configuration calls two Services
// with two policies. The Service of the fail-closed webhook is healthy;
// the one behind the webhook that ignores failure does not exist. Each
// webhook is tiered by its own policy, so this is a digest line, not a
// notification about a webhook that blocks.
func webhookIgnoreBesideClosed() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-ignore-beside-closed",
			Description: "A configuration with a healthy fail-closed " +
				"webhook and an ignoring webhook whose Service is " +
				"gone: a digest line only.",
			Root: "validatingwebhookconfiguration//mixed-policy",
			Tier: "digest", MaxMessages: 2, Tail: duration(time.Hour),
			MustNotBlame: []string{"deployment/guard-system/guard"},
		},
		build: buildWebhookIgnoreBesideClosed,
	}
}

func buildWebhookIgnoreBesideClosed(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	guard := c.deployment("guard-system", "guard",
		"registry.example.com/guard:1", 2)
	c.list(guard.objects())
	pods := []*corev1.Pod{guard.pod(0, "n1"), guard.pod(1, "n2")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "guard-system", "guard", 443))
	c.list(clusterSlice(c, "guard-system", "guard", pods...))
	config := clusterValidatingHook(c, "mixed-policy",
		"validate.guard.example.com", "guard-system", "guard")
	second := *config.Webhooks[0].DeepCopy()
	second.Name = "validate.widget.example.com"
	second.ClientConfig.Service.Name = c.n("widget-gone")
	ignore := admissionv1.Ignore
	second.FailurePolicy = &ignore
	config.Webhooks = append(config.Webhooks, second)
	c.list(config)
	c.after(30 * time.Minute)
}
