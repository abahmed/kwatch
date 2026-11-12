package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// oldProblemScenarios are the leftovers of a removed operator, problems
// that were already old when kwatch first saw them, and their young
// twins.
func oldProblemScenarios() []scenario {
	return []scenario{
		operatorLeftoversGrouped(), oldLeftoversAgeOut(),
		chronicIssuerLowered(), freshIssuerNotifies(),
		oldLeftoverJoinsLateGroup(),
	}
}

// oldLeftoverJoinsLateGroup: a scaled-to-zero operator left a
// fail-closed webhook behind, which is told on its own, and one
// certificate that expired over a year ago. Alone the certificate is
// an old problem and opens no incident. Ten minutes later a second
// certificate of the operator is found expired: the two are one group
// of leftovers, and the group names both, the old one included.
func oldLeftoverJoinsLateGroup() scenario {
	return scenario{
		expect: expectation{
			Name: "old-leftover-joins-late-group",
			Description: "A scaled-to-zero operator left a fail-closed " +
				"webhook and a certificate that expired over a year " +
				"ago; later a second expired certificate appears: the " +
				"two certificates are one digest incident.",
			Root: "validatingwebhookconfiguration//policy-resource",
			Tier: "notify", MaxMessages: 2, Tail: duration(time.Hour),
			OtherRoots: []string{
				"deployment/policy-system/policy-controller"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			operator := c.deployment("policy-system", "policy-controller",
				"registry.example.com/policy-controller:1.0", 0)
			c.list(operator.objects())
			c.list(policyService(c))
			c.list(clusterSlice(c, "policy-system", "policy-controller"))
			c.list(clusterValidatingHook(c, "policy-resource",
				"validate.policy-resource.example.com", "policy-system",
				"policy-controller"))
			c.list(agedTLSSecret(c, "policy-system", "policy-tls",
				400*24*time.Hour))
			c.after(10 * time.Minute)
			c.create(released(agedTLSSecret(c, "policy-system",
				"cleanup-tls", 24*time.Hour)))
			c.after(time.Minute)
		},
	}
}

// operatorLeftoversGrouped: a policy operator was scaled to zero. Its
// two webhooks ignore failure and its two serving certificates expired:
// four objects, one incident rooted at the operator, in the digest. A
// second operator in the same namespace, also scaled to zero, left its
// own webhook and certificate: they are its own group, not the policy
// operator's.
func operatorLeftoversGrouped() scenario {
	return scenario{
		expect: expectation{
			Name: "operator-leftovers-grouped",
			Description: "Two scaled-to-zero operators in one namespace " +
				"left webhooks and expired certificates behind: one " +
				"digest incident per operator, not one per object " +
				"nor one for the namespace.",
			Root: "deployment/policy-system/policy-controller",
			OtherRoots: []string{
				"deployment/policy-system/trace-operator"},
			Tier: "digest", MaxMessages: 2, Tail: duration(time.Hour),
			MustNotBlame: []string{"deployment/shop/orders"},
		},
		build: buildOperatorLeftovers,
	}
}

func buildOperatorLeftovers(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	operator := c.deployment("policy-system", "policy-controller",
		"registry.example.com/policy-controller:1.0", 0)
	c.list(operator.objects())
	c.list(policyService(c))
	c.list(clusterSlice(c, "policy-system", "policy-controller"))
	for _, name := range []string{"policy-resource", "policy-cleanup"} {
		c.list(ignoringHook(clusterValidatingHook(c, name,
			"validate."+name+".example.com", "policy-system",
			"policy-controller")))
	}
	c.list(agedTLSSecret(c, "policy-system", "policy-tls", 24*time.Hour))
	c.list(released(agedTLSSecret(c, "policy-system", "cleanup-tls",
		24*time.Hour)))
	listTraceOperator(c)
	orders := c.deployment("shop", "orders", "registry.example.com/o:2", 1)
	c.list(orders.objects())
	c.list(orders.pod(0, "n1"))
	c.after(time.Minute)
}

// oldLeftoversAgeOut: an expired certificate nothing uses and a
// VolumeAttachment whose volume is gone, both left over for more than a
// year before kwatch started. Neither changes by itself: no incident,
// only the count of old problems. Their young twins are
// certificate-expired-unused and attachment-volume-gone.
func oldLeftoversAgeOut() scenario {
	return scenario{
		expect: expectation{
			Name: "old-leftovers-age-out",
			Description: "An expired certificate nothing uses and a " +
				"stuck VolumeAttachment, both over a year old when " +
				"kwatch starts: no incident.",
			Quiet: true, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(agedTLSSecret(c, "shop", "legacy-tls",
				400*24*time.Hour))
			c.list(attachmentDeleted(c, "csi-9f8e7d", "pvc-9999", "n1",
				400*24*time.Hour))
			c.after(time.Minute)
		},
	}
}

// chronicIssuerLowered: a certificate issuer has been failing for two
// and a half years when kwatch starts. Nothing depends on it now and
// nothing changes: it is Low, not Needs attention.
func chronicIssuerLowered() scenario {
	return scenario{
		expect: expectation{
			Name: "chronic-issuer-lowered",
			Description: "A certificate issuer has reported Ready=False " +
				"for two and a half years: Low, with the reason.",
			Root: "issuer.certs.example.com/certs/letsencrypt",
			Tier: "digest", MaxMessages: 2, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(failingIssuer(c, 30*30*24*time.Hour))
			c.after(time.Minute)
		},
	}
}

// freshIssuerNotifies is the twin: the same issuer broke an hour ago.
// A new failure keeps its tier.
func freshIssuerNotifies() scenario {
	return scenario{
		expect: expectation{
			Name: "fresh-issuer-notifies",
			Description: "A certificate issuer reports Ready=False " +
				"since an hour ago: Needs attention.",
			Root: "issuer.certs.example.com/certs/letsencrypt",
			Tier: "notify", MaxMessages: 2, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(failingIssuer(c, time.Hour))
			c.after(time.Minute)
		},
	}
}

// agedTLSSecret is a TLS Secret in namespace whose certificate ended
// age before the scenario starts.
func agedTLSSecret(
	c *cluster, namespace, name string, age time.Duration,
) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: c.meta(namespace, name),
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("aged-crt"),
			corev1.TLSPrivateKeyKey: []byte("aged-key"),
		},
	}
	// The informer transform replaces the certificate with this
	// annotation; the scenario delivers what kwatch caches.
	secret.Annotations = map[string]string{
		"kwatch.dev/tls-not-after": c.start.Add(-age).UTC().
			Format(time.RFC3339),
	}
	return secret
}

// failingIssuer is a certificate issuer that has reported Ready=False
// for age.
func failingIssuer(c *cluster, age time.Duration) *unstructured.Unstructured {
	meta := clusterMeta(c, "certs", "letsencrypt", "issuer")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "certs.example.com/v1", "kind": "Issuer",
		"spec":   map[string]any{"server": "https://acme.example.com"},
		"status": map[string]any{"observedGeneration": int64(1)},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	clusterCondition(u, "Ready", "False", "ErrRegisterAccount",
		"Failed to register ACME account: 403 forbidden",
		c.start.Add(-age))
	return u
}

// policyService is the policy operator's webhook Service, labelled
// with its installation as a Helm chart labels it.
func policyService(c *cluster) *corev1.Service {
	return released(clusterService(c, "policy-system",
		"policy-controller", 443))
}

// released labels obj with the policy operator's installation.
func released[T interface{ SetLabels(map[string]string) }](obj T) T {
	obj.SetLabels(map[string]string{
		"app.kubernetes.io/instance": "policy"})
	return obj
}

// listTraceOperator lists a second operator in policy-system, scaled
// to zero, with one ignoring webhook and an expired certificate named
// after it.
func listTraceOperator(c *cluster) {
	operator := c.deployment("policy-system", "trace-operator",
		"registry.example.com/trace-operator:1.0", 0)
	c.list(operator.objects())
	c.list(clusterService(c, "policy-system", "trace-operator", 443))
	c.list(clusterSlice(c, "policy-system", "trace-operator"))
	c.list(ignoringHook(clusterValidatingHook(c, "trace-mutation",
		"mutate.trace.example.com", "policy-system", "trace-operator")))
	c.list(agedTLSSecret(c, "policy-system", "trace-operator-cert",
		24*time.Hour))
}
