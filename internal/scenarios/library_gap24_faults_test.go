package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gap24FaultScenarios: workload and configuration faults.
func gap24FaultScenarios() []scenario {
	return []scenario{
		gap24OOMLimit(), gap24ImageTypo(), gap24BadRolloutKey(),
		gap24ProbePort(), gap24CertificateExpired(),
	}
}

func gap24OOMLimit() scenario {
	return scenario{
		expect: g24Exp("wording-oom-limit-too-low",
			"Both replicas with a 256Mi limit are OOMKilled seven times "+
				"in twenty minutes on healthy nodes; the message names "+
				"the limit and one action.",
			"deployment/shop/api", "notify", 1, "node//n1", "node//n2"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:2", 2)
			configTemplate(w, lifecycleMemoryLimit("256Mi"))
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			for restarts := int32(1); restarts <= 7; restarts++ {
				c.after(3 * time.Minute)
				for i, node := range []string{"n1", "n2"} {
					c.update(w.pod(i, node, crashLoop(137, "OOMKilled", "",
						restarts)))
				}
				if restarts == 1 {
					w.setReady(0)
					c.update(w.objects())
				}
			}
		},
	}
}

func gap24ImageTypo() scenario {
	image := "registry.example/worker:v2.1O"
	return scenario{
		expect: g24Exp("wording-image-typo",
			"A rollout names an image tag with a letter O for a zero; "+
				"the registry answers manifest unknown. The message "+
				"quotes the exact image.",
			"deployment/billing/worker", "notify", 1,
			"registry//registry.example"),
		build: func(c *cluster) {
			w := g24Roll(c, "billing", "worker", "registry.example/worker:v2.1",
				2, nil, g24Image(image))
			for n := range 6 {
				clusterPullFailure(c, w, 0, "n1", n, image+
					": manifest unknown")
				c.after(40 * time.Second)
			}
		},
	}
}

func gap24BadRolloutKey() scenario {
	return scenario{
		expect: g24Exp("wording-bad-rollout-config-key",
			"A new revision stops at start on a missing config key "+
				"while the previous one still serves; the message says "+
				"which version is safe and quotes the failure.",
			"deployment/shop/api", "notify", 1, "node//n1"),
		build: func(c *cluster) {
			w := g24Roll(c, "shop", "api", "registry.example.com/api:2.2", 3,
				nil, g24Image("registry.example.com/api:2.3"))
			g2Crash(1, "Error", "panic: missing key DB_PASSWORD_V2")(c, w)
		},
	}
}

func gap24ProbePort() scenario {
	return scenario{
		expect: g24Exp("wording-probe-port-mismatch",
			"A release points the readiness probe at port 80 while the "+
				"container serves 8080; no new pod becomes ready. The "+
				"message names both ports.",
			"deployment/shop/web", "notify", 1, "node//n1", "node//n2"),
		build: func(c *cluster) {
			w := g24Roll(c, "shop", "web", "registry.example.com/web:5", 3,
				func(spec *corev1.PodSpec) { configProbe(spec, 8080) },
				func(spec *corev1.PodSpec) { configProbe(spec, 80) })
			g2Unready("Readiness probe failed: Get \"http://10.244.0.9:80/"+
				"healthz\": dial tcp 10.244.0.9:80: connect: connection "+
				"refused")(c, w)
		},
	}
}

func gap24CertificateExpired() scenario {
	return scenario{
		expect: g24Exp("wording-certificate-expired-handshake",
			"A mounted client certificate expired a day ago; the pods "+
				"crash on an expired-certificate handshake. The message "+
				"is in the past tense and names the Secret.",
			"secret/shop/bank-client-tls", "notify", 1,
			"deployment/shop/payments", "node//n1"),
		build: func(c *cluster) {
			w := certificateFleet(c, c.start.Add(-24*time.Hour))
			c.after(time.Minute)
			message := "Error: POST https://api.bank.example/v2/charge: " +
				"remote error: tls: expired certificate"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), w.pod(1, "n1", crashLoop(1, "Error",
					message, restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(45 * time.Second)
			}
		},
	}
}
