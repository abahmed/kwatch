package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// configExposureScenarios are config edits whose effect is far from
// them in time: a failure that outlasts the causal window, and an edit
// nothing reads until pods start hours later.
func configExposureScenarios() []scenario {
	return []scenario{
		configChangeLongCrashLoop(), secretKeyRemovedHoursEarlier(),
		secretKeyNeverPresent(), secretEditedHoursEarlierOwnCrash(),
	}
}

// configChangeLongCrashLoop: the configmap-change story with the crash
// loop simply continuing for most of an hour. The edit stays the cause
// however long the pods keep failing.
func configChangeLongCrashLoop() scenario {
	return scenario{
		expect: expectation{
			Name: "config-change-long-crash-loop",
			Description: "A ConfigMap edit breaks the mounted app.yaml " +
				"and every pod crash-loops for 50 minutes; the edit is " +
				"still the cause after the causal window has passed.",
			Root: "configmap/web/frontend-config", Tier: "notify",
			MaxMessages: 3,
			MustNotBlame: []string{"deployment/web/frontend", "node//n1",
				"node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			cm := configMap(c, "web", "frontend-config",
				map[string]string{"app.yaml": "max_connections: 100"})
			w := c.deployment("web", "frontend",
				"registry.example.com/frontend:7.2", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cm.Name,
							},
						},
					},
				}}
			})
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(90 * time.Second)
			edited := last(c, cm)
			edited.Data["app.yaml"] = "max_connections: unlimited"
			c.update(edited)
			c.after(50 * time.Second)
			crash := "config: /etc/frontend/app.yaml: max_connections: " +
				"cannot unmarshal !!str `unlimited` into int"
			for restarts := int32(2); restarts <= 50; restarts += 2 {
				for i := range 2 {
					c.update(w.pod(i, "n"+itoa(i+1),
						crashLoop(1, "Error", crash, restarts)))
				}
				w.setReady(0)
				c.update(w.deployment, w.replicaSet)
				c.after(2 * time.Minute)
			}
		},
	}
}

// keyedApp is a two-replica Deployment reading two keys of secret from
// its environment, listed running on two nodes.
func keyedApp(c *cluster, secret *corev1.Secret) *workload {
	w := c.deployment("shop", "orders",
		"registry.example.com/orders:4.1", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{
			configSecretEnv("DB_USER", secret.Name, "db-user"),
			configSecretEnv("DB_PASSWORD", secret.Name, "db-password"),
		}
	})
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), secret)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	return w
}

// keyedScaleUp adds two replicas whose containers end in state.
func keyedScaleUp(c *cluster, w *workload, state podState) {
	setReplicas(w, 4)
	w.setReady(2)
	c.update(w.deployment, w.replicaSet)
	for i := 2; i < 4; i++ {
		c.create(w.pod(i, "n"+itoa(i-1), startedNow, state))
	}
}

// secretKeyRemovedHoursEarlier: the secret-key-removed story with two
// hours between the edit and the first pods that read the Secret. An
// environment variable is read when a container starts, so nothing
// breaks until then; the edit is still the cause.
func secretKeyRemovedHoursEarlier() scenario {
	return scenario{
		expect: expectation{
			Name: "secret-key-removed-hours-earlier",
			Description: "A Secret edit removes a key the app reads from " +
				"its environment; two hours later new replicas fail " +
				"with CreateContainerConfigError naming the key.",
			Root: "secret/shop/db-creds", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders", "zone//zone-a",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			secret := configSecret(c, "shop", "db-creds",
				"db-user", "db-password")
			w := keyedApp(c, secret)
			c.after(2 * time.Minute)
			edited := last(c, secret)
			delete(edited.Data, "db-password")
			edited.Data["db-pass"] = []byte("rotated")
			c.update(edited)
			c.after(2 * time.Hour)
			message := "couldn't find key db-password in Secret " +
				secret.Namespace + "/" + secret.Name
			keyedScaleUp(c, w,
				waiting("CreateContainerConfigError", message))
			configWarn(c, w, 2, 4, "Failed", "Error: "+message)
		},
	}
}

// secretKeyNeverPresent: the Secret never held the key the pods read,
// and nothing edited it. The first pods to start since the reference was
// added cannot start; the kubelet names the Secret and the key, which
// is enough to blame the Secret without a change.
func secretKeyNeverPresent() scenario {
	return scenario{
		expect: expectation{
			Name: "secret-key-never-present",
			Description: "New replicas read a key that the Secret they " +
				"name never held; they fail with " +
				"CreateContainerConfigError naming the Secret and key.",
			Root: "secret/shop/db-creds", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders", "zone//zone-a",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			secret := configSecret(c, "shop", "db-creds", "db-user")
			w := keyedApp(c, secret)
			c.after(5 * time.Minute)
			message := "couldn't find key db-password in Secret " +
				secret.Namespace + "/" + secret.Name
			keyedScaleUp(c, w,
				waiting("CreateContainerConfigError", message))
			configWarn(c, w, 2, 4, "Failed", "Error: "+message)
		},
	}
}

// secretEditedHoursEarlierOwnCrash: a Secret gains a key, harmlessly.
// Two hours later new replicas crash on a panic of their own that has
// nothing to do with it. The old edit must not be blamed.
func secretEditedHoursEarlierOwnCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "secret-edited-hours-earlier-own-crash",
			Description: "A Secret gains a key; two hours later new " +
				"replicas crash-loop on their own panic, which does " +
				"not involve the Secret.",
			Root: "deployment/shop/orders", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"secret/shop/db-creds", "zone//zone-a",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			secret := configSecret(c, "shop", "db-creds",
				"db-user", "db-password")
			w := keyedApp(c, secret)
			c.after(2 * time.Minute)
			edited := last(c, secret)
			edited.Data["db-replica"] = []byte("added")
			c.update(edited)
			c.after(2 * time.Hour)
			crash := "panic: runtime error: index out of range [3] " +
				"with length 3"
			for _, restarts := range []int32{2, 4, 6} {
				if restarts == 2 {
					keyedScaleUp(c, w, crashLoop(2, "Error", crash, 2))
				} else {
					for i := 2; i < 4; i++ {
						c.update(w.pod(i, "n"+itoa(i-1), startedNow,
							crashLoop(2, "Error", crash, restarts)))
					}
				}
				c.after(2 * time.Minute)
			}
		},
	}
}
