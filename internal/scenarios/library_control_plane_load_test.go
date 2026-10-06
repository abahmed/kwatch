package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// controlPlaneLoadScenarios are an API server that is slow, read from its
// own /metrics, and the admission webhooks that make it slow.
func controlPlaneLoadScenarios() []scenario {
	return []scenario{
		apiServerWritesSlow(), slowWebhookSlowsAPI(),
		slowWebhookBlocksDeploys(),
	}
}

// apiLoad is one probe round with the API server's own numbers.
func apiLoad(c *cluster, attrs map[string]inventory.Value) {
	attrs[kube.AttrHealthy] = inventory.Bool(true)
	attrs[kube.AttrLatencyMS] = inventory.Number(40)
	c.emit(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ProbeSource, At: c.now,
		Entity: kube.APIServer, Attributes: attrs,
	})
}

// hookLoad is one round of the call statistics of a webhook
// configuration, as the prober writes them.
func hookLoad(c *cluster, name, hook string, p99, closed float64) {
	c.emit(inventory.Observation{
		Kind: inventory.Observed, Source: "webhook-metrics", At: c.now,
		Entity: inventory.CoreID(kube.KindValidatingHook, "", name),
		Attributes: map[string]inventory.Value{
			kube.AttrWebhookP99:         inventory.Number(p99),
			kube.AttrWebhookCalls:       inventory.Number(90),
			kube.AttrWebhookSlowest:     inventory.Text(hook),
			kube.AttrWebhookClosedShare: inventory.Number(closed),
			kube.AttrWebhookOpenShare:   inventory.Number(0),
		},
	})
}

// policyWebhook is a fail-closed validating webhook whose backend is
// healthy: two ready pods behind a Service. Only its calls are slow.
func policyWebhook(c *cluster) {
	backend := c.deployment("policy", "check",
		"registry.example.com/policy-check:1.4", 2)
	admissionServes(backend, 443)
	c.list(backend.objects())
	pods := []*corev1.Pod{backend.pod(0, "n1"), backend.pod(1, "n1")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "policy", "check", 443))
	c.list(admissionSlice(c, "policy", "check", 443, pods...))
	c.list(clusterValidatingHook(c, "policy-check",
		"check.policy.example.com", "policy", "check"))
}

func writesSlow(p99 float64, etcd float64) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrWritesP99:     inventory.Number(p99),
		kube.AttrWritesCalls:   inventory.Number(240),
		kube.AttrWritesSlowest: inventory.Text("create pods"),
		kube.AttrEtcdP99:       inventory.Number(etcd),
	}
}

// apiServerWritesSlow: the API server's writes take 2.4s at the 99th
// percentile for ten minutes, and its calls to etcd are as slow. Nothing
// else is failing. It is a line in the digest, not a page.
func apiServerWritesSlow() scenario {
	return scenario{
		expect: expectation{
			Name: "apiserver-writes-slow",
			Description: "The API server's writes stay slow for ten " +
				"minutes because etcd is slow; nothing else fails.",
			Root: "apiserver//kube-apiserver", Tier: "digest",
			MaxMessages: 2, Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			for range 20 {
				apiLoad(c, writesSlow(2400, 2100))
				c.after(30 * time.Second)
			}
		},
	}
}

// slowWebhookSlowsAPI: a validating webhook takes six seconds per call
// and answers every one, so no create fails; the API server's writes are
// slow because they wait for it. The webhook is the root, not the API
// server.
func slowWebhookSlowsAPI() scenario {
	return scenario{
		expect: expectation{
			Name: "slow-webhook-slows-api",
			Description: "A validating webhook answers in six seconds; " +
				"the API server's writes are slow and nothing fails.",
			Root: "validatingwebhookconfiguration//policy-check",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"apiserver//kube-apiserver"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			policyWebhook(c)
			for range 20 {
				apiLoad(c, writesSlow(6500, 30))
				hookLoad(c, "policy-check", "check.policy.example.com",
					6200, 0)
				c.after(30 * time.Second)
			}
		},
	}
}

// slowWebhookBlocksDeploys: the same webhook is so slow that a share of
// its calls run into the API server's deadline and, failing closed,
// refuse the request. A Deployment scaling up cannot create its pods.
func slowWebhookBlocksDeploys() scenario {
	return scenario{
		expect: expectation{
			Name: "slow-webhook-blocks-deploys",
			Description: "A fail-closed webhook takes eight seconds per " +
				"call and 40% of its calls hit the deadline; a " +
				"Deployment cannot create pods.",
			Root: "validatingwebhookconfiguration//policy-check",
			Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"apiserver//kube-apiserver",
				"deployment/shop/cart", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			policyWebhook(c)
			app := c.deployment("shop", "cart",
				"registry.example.com/cart:4", 3)
			c.list(app.objects())
			c.list(app.pod(0, "n1"), app.pod(1, "n2"),
				app.pod(2, "n1"))
			for range 6 {
				apiLoad(c, writesSlow(8800, 30))
				hookLoad(c, "policy-check", "check.policy.example.com",
					8200, 40)
				c.after(30 * time.Second)
			}
			setReplicas(app, 5)
			app.setReady(3)
			c.update(app.objects())
			message := "Internal error occurred: failed calling " +
				"webhook \"check.policy.example.com\": failed to " +
				"call webhook: Post \"https://" + c.n("check") + "." +
				c.n("policy") + ".svc:443/check?timeout=10s\": " +
				"context deadline exceeded"
			for range 8 {
				apiLoad(c, writesSlow(8800, 30))
				hookLoad(c, "policy-check", "check.policy.example.com",
					8200, 40)
				admissionFailedCreates(c, []*workload{app}, message, 1,
					0)
				c.after(30 * time.Second)
			}
		},
	}
}
