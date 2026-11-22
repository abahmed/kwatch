package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// gap17ConfigScenarios: a shared Secret, ConfigMap, issuer, store or
// webhook backend of another namespace breaks consumers across namespaces.
func gap17ConfigScenarios() []scenario {
	return []scenario{
		gap17IssuerSecretDeleted(), gap17IssuerSecretExpired(),
		gap17SharedConfigBadJSON(), gap17CorefileForwarder(),
		gap17ConversionWebhook(), gap17SecretStoreCredential(),
	}
}

const g17CertAPI = "certs.example.com/v1"

// g17Cert is a Certificate of namespace signed by issuer.
func g17Cert(
	c *cluster, namespace, name, issuer string,
) *unstructured.Unstructured {
	return g17Obj(c, g17CertAPI, "Certificate", namespace, name,
		map[string]any{"spec": map[string]any{"secretName": c.n(name),
			"issuerRef": map[string]any{"kind": "ClusterIssuer",
				"name": c.n(issuer)}}})
}

// g17CAStory plays a CA issuer whose key pair Secret breaks: fault edits
// the Secret (or removes it) and says what the issuer then reports.
func g17CAStory(
	c *cluster, namespaces []string,
	fault func(*corev1.Secret) (reason, message string, keep bool),
) {
	c.list(c.node("n1", "zone-a"))
	secret := configSecret(c, "cert-system", "internal-ca-keypair")
	secret.Type = corev1.SecretTypeTLS
	secret.CreationTimestamp = metav1.NewTime(c.start.Add(-41 * 24 * time.Hour))
	issuer := func(status, reason, message string) *unstructured.Unstructured {
		u := g17Obj(c, g17CertAPI, "ClusterIssuer", "", "internal-ca",
			map[string]any{"spec": map[string]any{"ca": map[string]any{
				"secretName": c.n("internal-ca-keypair")}}})
		clusterCondition(u, "Ready", status, reason, message, c.now)
		return u
	}
	c.list(secret, issuer("True", "KeyPairVerified", "Signing CA verified"))
	var certs []*unstructured.Unstructured
	for _, namespace := range namespaces {
		cert := g17Cert(c, namespace, "web-tls", "internal-ca")
		certs = append(certs, cert)
		c.list(cert)
	}
	app := g17Up(c, namespaces[0], "web", 2)
	c.after(10 * time.Minute)
	reason, message, keep := fault(secret)
	if keep {
		c.update(secret)
	} else {
		c.remove(secret)
	}
	c.update(issuer("False", reason, message))
	c.after(15 * time.Minute)
	for _, cert := range certs {
		clusterCondition(cert, "Ready", "False", "Issuing",
			"Issuing certificate as Secret does not exist", c.now)
		c.update(cert)
	}
	g17Loop(c, 6, func(round int32) {
		g17Mount(c, app, round, "MountVolume.SetUp failed for volume "+
			"\"tls\": secret \""+c.n("web-tls")+"\" not found")
	})
}

func gap17IssuerSecretDeleted() scenario {
	return scenario{
		expect: g2Exp("clusterissuer-secret-deleted-certificates-stalled",
			"A ClusterIssuer's CA key pair Secret is deleted by a "+
				"cleanup job; its Ready condition names the missing "+
				"Secret and Certificates of three namespaces stay "+
				"Issuing. Only metadata is read.",
			"clusterissuer.certs.example.com//internal-ca", "notify",
			"certificate.certs.example.com/shop/web-tls",
			"certificate.certs.example.com/billing/web-tls",
			"certificate.certs.example.com/reports/web-tls").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			g17CAStory(c, []string{"shop", "billing", "reports"},
				func(*corev1.Secret) (string, string, bool) {
					return "ErrGetKeyPair", "secret \"" +
						c.n("internal-ca-keypair") + "\" not found", false
				})
		},
	}
}

func gap17IssuerSecretExpired() scenario {
	return scenario{
		expect: g2Exp("clusterissuer-ca-secret-annotation-expired",
			"The certificate in the CA key pair Secret of an issuer "+
				"expired a day ago and the issuer reports an expired "+
				"certificate; Certificates of four namespaces are not "+
				"Ready. Read from the public certificate only.",
			"clusterissuer.certs.example.com//internal-ca", "notify",
			"certificate.certs.example.com/shop/web-tls",
			"certificate.certs.example.com/billing/web-tls").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			g17CAStory(c, []string{"shop", "billing", "reports", "jobs"},
				func(secret *corev1.Secret) (string, string, bool) {
					secret.Annotations = map[string]string{
						"kwatch.dev/tls-not-after": c.now.Add(
							-24 * time.Hour).UTC().Format(time.RFC3339)}
					return "ErrInvalidKeyPair", "x509: certificate has " +
						"expired", true
				})
		},
	}
}

func gap17SharedConfigBadJSON() scenario {
	return scenario{
		expect: g2Exp("shared-configmap-bad-json-three-namespaces",
			"A ConfigMap of a platform namespace is edited to invalid "+
				"JSON; pods of three namespaces rolled after the edit "+
				"crash naming it, while older pods keep running.",
			"unknown", "notify",
			"deployment/shop/api", "deployment/billing/api",
			"deployment/reports/api").budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			flags := &corev1.ConfigMap{
				ObjectMeta: c.meta("platform", "shared-flags"),
				Data:       map[string]string{"flags.json": "{\"a\": true}"}}
			c.list(flags)
			apps := g17Apps(c, 3, "shop/api", "billing/api", "reports/api")
			c.after(10 * time.Minute)
			flags = last(c, flags)
			flags.Data["flags.json"] = "{\"a\": tru"
			c.update(flags)
			c.after(time.Minute)
			g17Loop(c, 6, func(round int32) {
				for _, app := range apps {
					g17State(c, app, 2, crashLoop(1, "Error",
						"unexpected end of JSON input reading "+
							c.n("platform")+"/"+c.n("shared-flags")+
							" key flags.json", round))
				}
			})
		},
	}
}

func gap17CorefileForwarder() scenario {
	return scenario{
		expect: g2Exp("corefile-forwarder-edit-external-lookups-fail",
			"The CoreDNS ConfigMap is edited to forward to a dead "+
				"resolver while CoreDNS pods stay Ready; in-cluster "+
				"names resolve and external lookups of two namespaces "+
				"time out.",
			"configmap/kube-system/coredns", "notify",
			"deployment/kube-system/coredns", "cluster-dns//cluster-dns",
			"deployment/shop/checkout", "deployment/billing/sync").
			budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			corefile := &corev1.ConfigMap{
				ObjectMeta: c.meta("kube-system", "coredns"),
				Data: map[string]string{
					"Corefile": ".:53 {\n  forward . /etc/resolv.conf\n}"}}
			c.list(corefile)
			g17UpSpec(c, "kube-system", "coredns", 2,
				func(spec *corev1.PodSpec) {
					spec.Volumes = []corev1.Volume{{Name: "config",
						VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.
							ConfigMapVolumeSource{LocalObjectReference: corev1.
							LocalObjectReference{Name: corefile.Name}}}}}
				})
			apps := g17Apps(c, 2, "shop/checkout", "billing/sync")
			c.after(10 * time.Minute)
			corefile = last(c, corefile)
			corefile.Data["Corefile"] = ".:53 {\n  forward . 10.9.9.9\n}"
			c.update(corefile)
			c.after(5 * time.Minute)
			g17Loop(c, 8, func(round int32) {
				g17Victims(c, apps, round,
					"lookup api.partner.example: i/o timeout")
			})
		},
	}
}

func gap17ConversionWebhook() scenario {
	return scenario{
		expect: g2Exp("crd-conversion-webhook-no-endpoints",
			"A CRD's conversion webhook is served by a Service whose "+
				"only pod cannot pull its image; controllers of two "+
				"namespaces fail reading the resource. The Deployment is "+
				"the root.",
			"deployment/crd-system/gadget-conv", "notify",
			"customresourcedefinition//gadgets.example.com",
			"deployment/shop/gadget-ctl", "deployment/billing/gadget-ctl").
			budget(2).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			crd := g17Obj(c, "apiextensions.k8s.io/v1",
				"CustomResourceDefinition", "", "gadgets.example.com",
				map[string]any{"spec": map[string]any{
					"group": "example.com", "conversion": map[string]any{
						"strategy": "Webhook", "webhook": map[string]any{
							"clientConfig": map[string]any{
								"service": map[string]any{
									"namespace": c.n("crd-system"),
									"name":      c.n("gadget-conv")}}}}}})
			c.list(crd)
			conv := g17Up(c, "crd-system", "gadget-conv", 1)
			apps := g17Apps(c, 2, "shop/gadget-ctl", "billing/gadget-ctl")
			c.after(10 * time.Minute)
			g17Loop(c, 6, func(round int32) {
				g17State(c, conv, 1, gapPending(waiting("ImagePullBackOff",
					"toomanyrequests: rate limit")))
				c.update(clusterSlice(c, "crd-system", "gadget-conv"))
				g17Victims(c, apps, round, "conversion webhook for "+
					"example.com/v1, Kind=Gadget failed: dial tcp: "+
					"connect: connection refused")
			})
		},
	}
}

func gap17SecretStoreCredential() scenario {
	return scenario{
		expect: g2Exp("secretstore-credential-secret-missing-externalsecrets-fail",
			"A cluster-scoped SecretStore references a credential "+
				"Secret that was deleted; ExternalSecrets of three "+
				"namespaces fail to sync and their pods wait on an app "+
				"Secret two hops downstream.",
			"unknown", "notify",
			"externalsecret.secrets.example.com/shop/db-creds",
			"externalsecret.secrets.example.com/billing/db-creds",
			"deployment/shop/api").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			token := configSecret(c, "platform", "store-token")
			store := g17Obj(c, "secrets.example.com/v1", "SecretStore", "",
				"platform-store", map[string]any{"spec": map[string]any{
					"secretRef": map[string]any{
						"namespace": c.n("platform"),
						"name":      c.n("store-token")}}})
			c.list(token, store)
			spaces := []string{"shop", "billing", "reports"}
			var apps []*workload
			var syncs []*unstructured.Unstructured
			for _, namespace := range spaces {
				sync := g17Obj(c, "secrets.example.com/v1", "ExternalSecret",
					namespace, "db-creds", map[string]any{
						"spec": map[string]any{"secretStoreRef": map[string]any{
							"name": c.n("platform-store")}}})
				syncs = append(syncs, sync)
				c.list(sync)
				apps = append(apps, g17Up(c, namespace, "api", 2))
			}
			c.after(10 * time.Minute)
			c.remove(token)
			c.after(30 * time.Minute)
			for _, sync := range syncs {
				clusterCondition(sync, "Ready", "False", "SecretSyncedError",
					"store not ready", c.now)
				c.update(sync)
			}
			g17Loop(c, 6, func(int32) {
				for _, app := range apps {
					g17State(c, app, 1, gapPending(waiting(
						"CreateContainerConfigError",
						"secret \""+c.n("db-creds")+"\" not found")))
				}
			})
		},
	}
}
