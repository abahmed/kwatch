package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// webhookDeniesPod: the image-policy webhook is healthy and answers
// every call, but it denies the pods of one Deployment because their
// image has no signature. That is a policy decision about one workload,
// not an outage: the webhook configuration is the cause, the tier is
// notify, and the unaffected Deployment is not blamed.
func webhookDeniesPod() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-denies-pod",
			Description: "A healthy validating webhook denies the pods " +
				"of one Deployment; the policy is the cause, not an " +
				"outage.",
			Root: "validatingwebhookconfiguration//image-policy",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"service/policy-system/image-policy",
				"deployment/policy-system/image-policy",
				"deployment/search/api", "node//n1"},
		},
		build: buildWebhookDenies,
	}
}

func buildWebhookDenies(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	hook := c.deployment("policy-system", "image-policy",
		"registry.example.com/image-policy:1.4", 2)
	admissionServes(hook, 443)
	c.list(hook.objects())
	pods := []*corev1.Pod{hook.pod(0, "n1"), hook.pod(1, "n2")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "policy-system", "image-policy", 443))
	c.list(admissionSlice(c, "policy-system", "image-policy", 443,
		pods...))
	c.list(clusterValidatingHook(c, "image-policy",
		"image-policy.example.com", "policy-system", "image-policy"))
	bad := c.deployment("payments", "checkout",
		"registry.example.com/checkout:unsigned", 2)
	good := c.deployment("search", "api", "registry.example.com/api:4", 2)
	for _, w := range []*workload{bad, good} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(2 * time.Minute)
	setReplicas(bad, 3)
	bad.setReady(2)
	c.update(bad.objects())
	message := "Internal error occurred: admission webhook " +
		"\"image-policy.example.com\" denied the request: image " +
		"registry.example.com/checkout:unsigned has no signature"
	admissionFailedCreates(c, []*workload{bad}, message, 4, 45*time.Second)
}
