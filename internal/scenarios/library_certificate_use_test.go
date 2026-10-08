package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// expiredTLSSecret lists a TLS Secret whose certificate ended a day
// ago.
func expiredTLSSecret(c *cluster, name string) {
	secret := &corev1.Secret{
		ObjectMeta: c.meta("shop", name),
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("digest-crt"),
			corev1.TLSPrivateKeyKey: []byte("digest-key"),
		},
	}
	// The informer transform replaces the certificate with this
	// annotation; the scenario delivers what kwatch caches.
	notAfter := c.start.Add(-24 * time.Hour)
	secret.Annotations = map[string]string{
		"kwatch.dev/tls-not-after": notAfter.UTC().Format(time.RFC3339),
	}
	c.list(secret)
}

// certificateExpiredUnused: a leftover certificate from an uninstalled
// operator ended yesterday and nothing references it. It breaks
// nothing, so it waits for the digest and never says clients reject it.
func certificateExpiredUnused() scenario {
	return scenario{
		expect: expectation{
			Name: "certificate-expired-unused",
			Description: "An expired TLS Secret that no pod, Ingress or " +
				"account references: a digest line, not an outage.",
			Root: "secret/shop/old-operator-tls", Tier: "digest",
			MaxMessages: 2, Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			expiredTLSSecret(c, "old-operator-tls")
			c.after(time.Minute)
		},
	}
}

// certificateExpiredIngress: the certificate an Ingress terminates TLS
// with ended yesterday. Browsers refuse the site, so it interrupts and
// says clients are rejecting it.
func certificateExpiredIngress() scenario {
	return scenario{
		expect: expectation{
			Name: "certificate-expired-ingress",
			Description: "An Ingress terminates TLS with a certificate " +
				"that has expired: clients refuse the site now.",
			Root: "secret/shop/shop-tls", Tier: "notify",
			MaxMessages: 2, Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			expiredTLSSecret(c, "shop-tls")
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			c.list(trafficIngress(c, "web", "shop-tls"))
			c.after(time.Minute)
		},
	}
}
