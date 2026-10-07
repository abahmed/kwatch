package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// webhookTLSScenarios are admission webhooks the API server cannot
// call because the TLS handshake fails.
func webhookTLSScenarios() []scenario {
	return []scenario{webhookUnknownAuthority(), webhookServingCertExpired()}
}

// webhookUnknownAuthority: the image-policy webhook (fail closed) has
// ready endpoints, but the API server does not trust the certificate it
// serves. Every pod creation it intercepts fails with an x509 error. The
// webhook configuration is the root and the error is quoted.
func webhookUnknownAuthority() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-tls-unknown-authority",
			Description: "A fail-closed validating webhook with ready " +
				"endpoints fails every call with an x509 unknown " +
				"authority error; pod creation fails in two " +
				"namespaces.",
			Root: "validatingwebhookconfiguration//image-policy",
			Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/payments/gateway",
				"deployment/search/api", "node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			webhookTLSFleet(c, nil,
				"x509: certificate signed by unknown authority")
		},
	}
}

// webhookServingCertExpired: the same failure, and the certificate the
// webhook's pods serve (Secret policy-tls) expired seconds before the first
// failure: a present fact the message adds.
func webhookServingCertExpired() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-tls-serving-cert-expired",
			Description: "A fail-closed validating webhook fails every " +
				"call with an expired certificate error; the Secret " +
				"its pods serve expired seconds before, and is its " +
				"own incident.",
			Root: "secret/policy-system/policy-tls",
			OtherRoots: []string{
				"validatingwebhookconfiguration//image-policy"},
			Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"deployment/payments/gateway",
				"deployment/search/api", "node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			expired := c.start.Add(time.Minute + 50*time.Second)
			webhookTLSFleet(c, &expired,
				"x509: certificate has expired or is not yet valid: "+
					"current time is after the certificate's end date")
		},
	}
}

// webhookTLSFleet lists the webhook, its two pods (serving from Secret
// policy-tls when notAfter is set) and two workloads whose pod creation
// then fails with the given TLS error.
func webhookTLSFleet(c *cluster, notAfter *time.Time, tlsError string) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	hook := c.deployment("policy-system", "image-policy",
		"registry.example.com/image-policy:1.4", 2)
	if notAfter != nil {
		c.list(servingSecret(c, *notAfter))
		configTemplate(hook, func(spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{{Name: "tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: c.n("policy-tls")},
				}}}
		})
	}
	admissionServes(hook, 443)
	c.list(hook.objects())
	pods := []*corev1.Pod{hook.pod(0, "n1"), hook.pod(1, "n2")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "policy-system", "image-policy", 443))
	c.list(admissionSlice(c, "policy-system", "image-policy", 443, pods...))
	c.list(clusterValidatingHook(c, "image-policy",
		"image-policy.example.com", "policy-system", "image-policy"))
	apps := []*workload{
		c.deployment("payments", "gateway", "registry.example.com/gw:8", 2),
		c.deployment("search", "api", "registry.example.com/search:4", 2),
	}
	for _, w := range apps {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(2 * time.Minute)
	message := "Internal error occurred: failed calling webhook " +
		"\"image-policy.example.com\": failed to call webhook: Post " +
		"\"https://" + c.n("image-policy") + "." + c.n("policy-system") +
		".svc:443/validate?timeout=10s\": " + tlsError
	for _, w := range apps {
		setReplicas(w, 3)
		w.setReady(2)
		c.update(w.objects())
	}
	admissionFailedCreates(c, apps, message, 4, 45*time.Second)
}

// servingSecret is the webhook's TLS Secret as kwatch caches it: only
// the expiry of its certificate is kept.
func servingSecret(c *cluster, notAfter time.Time) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: c.meta("policy-system", "policy-tls"),
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("digest-crt"),
			corev1.TLSPrivateKeyKey: []byte("digest-key"),
		},
	}
	secret.Annotations = map[string]string{
		"kwatch.dev/tls-not-after": notAfter.UTC().Format(time.RFC3339),
	}
	return secret
}
