package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// secretPullRegistry is the registry the image typo scenario pulls from.
const secretPullRegistry = "registry.corp.example"

// rcaEvidenceChangeScenarios are Secret edits that only sit near a
// crash: made after it began (a fix attempt), or made minutes before a
// crash whose words are a defect of the program.
func rcaEvidenceChangeScenarios() []scenario {
	return []scenario{
		secretEditedAfterCodeCrash(), secretRotatedBeforeCodeCrash(),
		imageTypoAfterSecretEdit(),
	}
}

// secretEditedAfterCodeCrash: orders panics on a code defect; eight
// minutes later an engineer edits the Secret it reads, trying to fix it.
// An edit made after the failures began did not cause them.
func secretEditedAfterCodeCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "secret-edited-after-code-crash",
			Description: "An app crashes on a code defect; an engineer " +
				"edits the Secret it reads eight minutes later, as a " +
				"fix attempt. The edit came after the failures.",
			Root: "deployment/shop/orders", Tier: "notify",
			MaxMessages:  4,
			MustNotBlame: []string{"secret/shop/db-creds"},
		},
		build: func(c *cluster) {
			secret := configSecret(c, "shop", "db-creds", "db-user",
				"db-password")
			w := c.deployment("shop", "orders",
				"registry.example.com/orders:4.1", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{
					configSecretEnv("DB_PASSWORD", secret.Name,
						"db-password")}
			})
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), secret)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(2 * time.Minute)
			for i := 0; i < 2; i++ {
				c.update(w.pod(i, "n"+itoa(i+1),
					crashLoop(2, "Error", rcaEvidencePanic, 4)))
			}
			w.setReady(0)
			c.update(w.objects())
			c.after(8 * time.Minute)
			edited := last(c, secret)
			edited.Data["db-password"] = []byte("rotated")
			c.update(edited)
			c.after(5 * time.Minute)
		},
	}
}

// secretRotatedBeforeCodeCrash: a TLS Secret is renewed; three minutes
// later orders panics on a code defect. Renewals happen all the time and
// a nil dereference is not what a certificate causes.
func secretRotatedBeforeCodeCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "secret-rotated-before-code-crash",
			Description: "A TLS Secret is renewed; three minutes later " +
				"an app that reads it panics on a code defect.",
			Root: "deployment/shop/orders", Tier: "notify",
			MaxMessages:  4,
			MustNotBlame: []string{"secret/shop/tls-cert"},
		},
		build: func(c *cluster) {
			secret := configSecret(c, "shop", "tls-cert", "tls.crt",
				"tls.key")
			w := c.deployment("shop", "orders",
				"registry.example.com/orders:4.1", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{
					configSecretEnv("TLS_CERT", secret.Name, "tls.crt")}
			})
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), secret)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(2 * time.Minute)
			edited := last(c, secret)
			edited.Data["tls.crt"] = []byte("renewed")
			c.update(edited)
			c.after(3 * time.Minute)
			for i := 0; i < 2; i++ {
				c.update(w.pod(i, "n"+itoa(i+1),
					crashLoop(2, "Error", rcaEvidencePanic, 4)))
			}
			w.setReady(0)
			c.update(w.objects())
			c.after(5 * time.Minute)
		},
	}
}

// imageTypoAfterSecretEdit: an environment Secret of checkout is edited;
// two minutes later the Deployment rolls out a tag the registry does not
// have. The kubelet never reads an environment Secret to pull an image,
// so the edit cannot be why the pull fails.
func imageTypoAfterSecretEdit() scenario {
	return scenario{
		expect: expectation{
			Name: "image-typo-after-secret-edit",
			Description: "A Deployment rolls out a mistyped image tag " +
				"two minutes after an environment Secret it reads was " +
				"edited; the pull error is about the image.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"secret/shop/db-creds",
				"registry//" + secretPullRegistry},
		},
		build: buildImageTypoAfterSecretEdit,
	}
}

func buildImageTypoAfterSecretEdit(c *cluster) {
	secret := configSecret(c, "shop", "db-creds", "db-password")
	checkout := c.deployment("shop", "checkout",
		secretPullRegistry+"/shop/checkout:1.9.0", 2)
	configTemplate(checkout, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{
			configSecretEnv("DB_PASSWORD", secret.Name, "db-password")}
	})
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), secret)
	c.list(checkout.objects())
	c.list(checkout.pod(0, "n1"), checkout.pod(1, "n2"))
	c.after(time.Minute)
	edited := last(c, secret)
	edited.Data["db-password"] = []byte("rotated")
	c.update(edited)
	c.after(2 * time.Minute)
	image := secretPullRegistry + "/shop/checkout:1.9.O"
	rs := checkout.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = image
	})
	checkout.setReady(2)
	c.update(checkout.deployment)
	c.create(rs)
	for n := range 6 {
		clusterPullFailure(c, checkout, 0, "n1", n, image+": not found")
		c.after(40 * time.Second)
	}
}
