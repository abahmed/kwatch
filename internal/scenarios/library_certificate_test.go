package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// certificateScenarios are TLS certificates that expire under the pods
// that use them, and the negative case of a certificate that is only
// close to expiry while its pods crash for another reason.
func certificateScenarios() []scenario {
	return []scenario{certificateExpired(), certificateExpiringAppCrash(),
		certificateExpiredUnused(), certificateExpiredIngress()}
}

// certificateExpired: the client certificate the payments pods present
// to their bank expired an hour ago; open connections kept working
// until the pods reconnect, and every new handshake is refused. The
// Secret holding the certificate is the root.
func certificateExpired() scenario {
	return scenario{
		expect: expectation{
			Name: "certificate-expired",
			Description: "A client TLS certificate mounted by a " +
				"Deployment has expired; its pods crash on refused " +
				"handshakes when they reconnect.",
			Root: "secret/shop/bank-client-tls", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/payments",
				"node//n1"},
		},
		build: func(c *cluster) {
			w := certificateFleet(c, c.start.Add(-time.Hour))
			c.after(time.Minute)
			message := "Error: POST https://api.bank.example/v2/charge: " +
				"remote error: tls: expired certificate"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), w.pod(1, "n1", crashLoop(1, "Error", message,
					restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(45 * time.Second)
			}
		},
	}
}

// certificateExpiringAppCrash: the same certificate has ten days left,
// which is digest material, while the payments pods crash on a nil map.
// The application is the root; the certificate must not be blamed.
func certificateExpiringAppCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "certificate-expiring-app-crash",
			Description: "A mounted TLS certificate expires in ten days " +
				"while its pods crash for an unrelated reason.",
			Root: "deployment/shop/payments", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"secret/shop/bank-client-tls"},
		},
		build: func(c *cluster) {
			w := certificateFleet(c, c.start.Add(10*24*time.Hour))
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(2, "Error",
					"panic: assignment to entry in nil map", restarts)),
					w.pod(1, "n1", crashLoop(2, "Error",
						"panic: assignment to entry in nil map", restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(45 * time.Second)
			}
		},
	}
}

// certificateFleet lists payments with two replicas that mount the
// bank-client-tls Secret, whose certificate expires at notAfter.
func certificateFleet(c *cluster, notAfter time.Time) *workload {
	c.list(c.node("n1", "zone-a"))
	secret := &corev1.Secret{
		ObjectMeta: c.meta("shop", "bank-client-tls"),
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("digest-crt"),
			corev1.TLSPrivateKeyKey: []byte("digest-key"),
		},
	}
	// The informer transform replaces the certificate with this
	// annotation; the scenario delivers what kwatch caches.
	secret.Annotations = map[string]string{
		"kwatch.dev/tls-not-after": notAfter.UTC().Format(time.RFC3339),
	}
	c.list(secret)
	w := c.deployment("shop", "payments", "registry.example.com/pay:7", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Volumes = []corev1.Volume{{Name: "bank-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: c.n("bank-client-tls")},
			}}}
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"))
	return w
}
