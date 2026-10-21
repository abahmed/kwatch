package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// advQuotaFullQuiet: a namespace's pod quota is exactly used up and
// nothing has tried to exceed it. A full quota with no refusal is not
// a failure.
func advQuotaFullQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-quota-full-no-refusal-quiet",
			Description: "A pod quota is used up exactly; every workload " +
				"is at its desired size and nothing is refused.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("batch", "worker",
				"registry.example.com/worker:3", 6)
			c.list(w.objects())
			for i := range 6 {
				c.list(w.pod(i, []string{"n1", "n2"}[i%2]))
			}
			c.list(advPodQuota(c, 6))
			c.after(30 * time.Minute)
		},
	}
}

// advSecretRotationQuiet: a Secret is rotated and the Deployment that
// reads it is restarted; every new pod is ready within twenty seconds.
func advSecretRotationQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-secret-rotation-healthy-quiet",
			Description: "A Secret is rotated and its Deployment restarts " +
				"cleanly.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			secret := configSecret(c, "shop", "payments-keys", "api-key")
			w := c.deployment("shop", "payments",
				"registry.example.com/payments:3", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{configSecretEnv(
					"API_KEY", secret.Name, "api-key")}
			})
			c.list(secret, w.deployment, w.replicaSet)
			old := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n2")}
			c.list(old[0], old[1])
			c.after(2 * time.Minute)
			rotated := last(c, secret)
			rotated.Data["api-key"] = []byte("rotated")
			c.update(rotated)
			c.after(10 * time.Second)
			advRestart(c, w)
			c.create(w.pod(0, "n1", startedNow, notReady),
				w.pod(1, "n2", startedNow, notReady))
			c.after(20 * time.Second)
			c.update(w.pod(0, "n1", startedNow), w.pod(1, "n2", startedNow))
			c.remove(old[0], old[1])
			w.setReady(2)
			c.update(w.objects())
		},
	}
}

// advQuotaFullQuietLong is advQuotaFullQuiet watched for two days, long
// enough for every digest to be sent.
func advQuotaFullQuietLong() scenario {
	s := advQuotaFullQuiet()
	s.expect.Name = "adv-quota-full-no-refusal-two-days"
	s.expect.Tail = duration(48 * time.Hour)
	s.expect.Description += " Watched for two days."
	return s
}

// advThrottledHealthyQuiet: containers are CPU-throttled 70% of the
// time but every probe passes and every pod is ready. Throttling that
// breaks nothing is not an outage.
func advThrottledHealthyQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-throttled-but-healthy-quiet",
			Description: "Pods are heavily CPU-throttled and still ready, " +
				"with no restarts, for two days.",
			Quiet: true, Tail: duration(48 * time.Hour),
		},
		build: func(c *cluster) {
			w := throttledWorkload(c)
			for range 10 {
				c.after(time.Minute)
				throttleReading(c, w, 70)
			}
		},
	}
}
