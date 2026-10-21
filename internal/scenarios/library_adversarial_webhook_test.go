package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

var advNamespaces = []string{"payments", "search", "billing", "shipping",
	"identity", "reports"}

// advWebhookTimeoutManyNamespaces: a fail-closed webhook with ready
// endpoints times out on every call and pod creation fails in six
// namespaces at once, each with its own ReplicaSet events.
func advWebhookTimeoutManyNamespaces() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-webhook-timeout-six-namespaces",
			Description: "A fail-closed validating webhook with ready " +
				"endpoints times out; ReplicaSets in six namespaces " +
				"cannot create pods.",
			Root: "validatingwebhookconfiguration//image-policy",
			Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/policy-system/image-policy",
				"service/policy-system/image-policy", "node//n1",
				"deployment/payments/app", "deployment/search/app",
				"deployment/billing/app", "deployment/shipping/app",
				"deployment/identity/app", "deployment/reports/app"},
		},
		build: func(c *cluster) {
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
			var apps []*workload
			for _, ns := range advNamespaces {
				w := c.deployment(ns, "app", "registry.example.com/"+ns+":1", 2)
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n2"))
				apps = append(apps, w)
			}
			c.after(2 * time.Minute)
			message := "Internal error occurred: failed calling webhook " +
				"\"image-policy.example.com\": failed to call webhook: " +
				"Post \"https://" + c.n("image-policy") + "." +
				c.n("policy-system") + ".svc:443/validate?timeout=10s\": " +
				"context deadline exceeded"
			for _, w := range apps {
				setReplicas(w, 3)
				w.setReady(2)
				c.update(w.objects())
			}
			for n := int32(1); n <= 4; n++ {
				for _, w := range apps {
					c.warn(c.warningEvent(w.replicaSet, "ReplicaSet",
						"FailedCreate", "Error creating: "+message,
						"replicaset-controller", n))
				}
				c.after(45 * time.Second)
			}
		},
	}
}
