package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gap7bCSRNotIssued: a certificate signing request was approved but no
// signer issues the certificate; the join it waits for stalls.
func gap7bCSRNotIssued() scenario {
	return scenario{
		expect: g2Exp("csr-approved-but-never-issued",
			"A certificate signing request is Approved but its status "+
				"never gets a certificate; nothing signs it.",
			"certificatesigningrequest//csr-9d4k1", "notify", "node//n1").
			tail(30 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			spec := map[string]any{"signerName": "kubernetes.io/kube-" +
				"apiserver-client-kubelet"}
			status := map[string]any{"conditions": csrCondition(c,
				"Approved", "AutoApproved", "Auto approving kubelet "+
					"client certificate")}
			csr := builtinObject(c, "certificates.k8s.io/v1",
				"CertificateSigningRequest", "", "csr-9d4k1", spec, status)
			c.create(csr)
			for range 4 {
				c.after(10 * time.Minute)
				c.update(csr.DeepCopy())
			}
		},
	}
}

// gap7bPolicyExpressionError: a ValidatingAdmissionPolicy's expression
// reads a field that is not there; under failurePolicy Fail every matching
// create errors.
func gap7bPolicyExpressionError() scenario {
	return scenario{
		expect: g2Exp("admission-policy-expression-error-denies-all",
			"A ValidatingAdmissionPolicy's expression errors on every "+
				"object (no such key); under failurePolicy Fail pod "+
				"creation is refused in three namespaces.",
			"validatingadmissionpolicy//require-team-label", "notify",
			"node//n1", "deployment/shop/orders").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(admissionPolicy(c, "require-team-label"))
			fleet := gap2Spread(c, []string{"shop/orders", "billing/ledger",
				"media/thumbs"}, []string{"n1", "n2"})
			c.after(10 * time.Minute)
			gap6Rollouts(c, fleet, "Error creating: pods is forbidden: "+
				"ValidatingAdmissionPolicy 'require-team-label' with "+
				"binding 'require-team-label-binding' denied request: "+
				"expression 'object.metadata.labels.team == \"core\"' "+
				"resulted in error: no such key: team", 6)
		},
	}
}

// gap7bDropAllCapabilities: a hardening release drops all capabilities;
// the entrypoint's chown of its data directory is refused.
func gap7bDropAllCapabilities() scenario {
	return g2Release(g2Exp("release-drops-all-capabilities-chown-fails",
		"A hardening release drops all capabilities; the entrypoint's "+
			"chown of its data directory fails with \"Operation not "+
			"permitted\" at every start.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].SecurityContext = &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{
					"ALL"}}}
		},
		g2Crash(1, "Error", "chown: changing ownership of '/var/lib/api': "+
			"Operation not permitted"))
}

// gap7bRunAsUserNoPasswd: a release runs the container as UID 1001, which
// the image's password file does not know.
func gap7bRunAsUserNoPasswd() scenario {
	return g2Release(g2Exp("release-run-as-user-without-passwd-entry",
		"A release runs the container as UID 1001, which the image's "+
			"password file lacks; the start-up tool fails to look the "+
			"user up.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			user := int64(1001)
			spec.Containers[0].SecurityContext = &corev1.SecurityContext{
				RunAsUser: &user}
		},
		g2Crash(1, "Error", "initdb: error: could not look up effective "+
			"user ID 1001: user does not exist"))
}
