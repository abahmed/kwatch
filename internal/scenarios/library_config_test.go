package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// configScenarios are failures caused by a Secret or ConfigMap the pods
// use.
func configScenarios() []scenario {
	return []scenario{
		secretKeyRemoved(), missingSecret(), configMapChange(),
	}
}

// secretKeyRemoved: an edit drops the db-password key from a Secret; the
// running pods keep their environment, but the replicas an autoscaler
// adds cannot start because the kubelet cannot resolve the key. The
// Secret edit is the cause.
func secretKeyRemoved() scenario {
	return scenario{
		expect: expectation{
			Name: "secret-key-removed",
			Description: "A Secret edit removes a key the app reads from " +
				"its environment; new replicas fail with " +
				"CreateContainerConfigError naming the key.",
			Root: "secret/shop/db-creds", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders", "zone//zone-a",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			secret := configSecret(c, "shop", "db-creds",
				"db-user", "db-password")
			w := c.deployment("shop", "orders",
				"registry.example.com/orders:4.1", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{
					configSecretEnv("DB_USER", secret.Name, "db-user"),
					configSecretEnv("DB_PASSWORD", secret.Name,
						"db-password"),
				}
			})
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), secret)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(2 * time.Minute)
			edited := last(c, secret)
			delete(edited.Data, "db-password")
			edited.Data["db-pass"] = []byte("rotated")
			c.update(edited)
			c.after(3 * time.Minute)
			setReplicas(w, 4)
			w.setReady(2)
			c.update(w.deployment, w.replicaSet)
			message := "couldn't find key db-password in Secret " +
				secret.Namespace + "/" + secret.Name
			for i := 2; i < 4; i++ {
				c.create(w.pod(i, "n"+itoa(i-1), startedNow,
					waiting("CreateContainerConfigError", message)))
			}
			configWarn(c, w, 2, 4, "Failed", "Error: "+message)
		},
	}
}

// missingSecret: a new Deployment references a Secret nobody created;
// every pod fails with CreateContainerConfigError. The absent Secret is
// the cause.
func missingSecret() scenario {
	return scenario{
		expect: expectation{
			Name: "missing-secret",
			Description: "A new Deployment's pods reference a Secret that " +
				"does not exist and fail with CreateContainerConfigError.",
			Root: "secret/billing/stripe-api", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2", "zone//zone-a",
				"registry//registry.example.com"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.after(time.Minute)
			w := c.deployment("billing", "invoicer",
				"registry.example.com/invoicer:1.0", 2)
			name := c.n("stripe-api")
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: name,
						},
					},
				}}
			})
			w.setReady(0)
			c.create(w.objects())
			message := "secret \"" + name + "\" not found"
			for i := range 2 {
				c.create(w.pod(i, "n"+itoa(i+1), startedNow,
					waiting("CreateContainerConfigError", message)))
			}
			configWarn(c, w, 0, 2, "Failed", "Error: "+message)
			c.after(4 * time.Minute)
			configWarn(c, w, 0, 2, "Failed", "Error: "+message)
		},
	}
}

// configMapChange: an edit to the app.yaml key of a ConfigMap is picked
// up by the running pods, which exit and crash-loop on the new value.
// The ConfigMap edit is the cause.
func configMapChange() scenario {
	return scenario{
		expect: expectation{
			Name: "configmap-change",
			Description: "A ConfigMap edit breaks the mounted app.yaml; " +
				"every pod reloads, exits and crash-loops with an " +
				"error naming the key.",
			Root: "configmap/web/frontend-config", Tier: "notify",
			MaxMessages: 2,
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
			for _, restarts := range []int32{2, 4} {
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

// configTemplate edits the pod template before anything is listed and
// rebuilds the ReplicaSet from it.
func configTemplate(w *workload, mutate func(*corev1.PodSpec)) {
	mutate(&w.deployment.Spec.Template.Spec)
	w.replicaSet = w.newReplicaSet()
	w.setReady(*w.deployment.Spec.Replicas)
}

func configSecret(c *cluster, namespace, name string,
	keys ...string) *corev1.Secret {
	data := map[string][]byte{}
	for _, key := range keys {
		data[key] = []byte("value-of-" + key)
	}
	return &corev1.Secret{
		ObjectMeta: c.meta(namespace, name), Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

func configMap(c *cluster, namespace, name string,
	data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: c.meta(namespace, name), Data: data}
}

func configSecretEnv(name, secret, key string) corev1.EnvVar {
	return corev1.EnvVar{Name: name, ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secret},
			Key:                  key,
		},
	}}
}

// configWarn records the kubelet's Warning event for replicas [from, to).
func configWarn(c *cluster, w *workload, from, to int, reason,
	message string) {
	for i := from; i < to; i++ {
		pod := last(c, w.pod(i, ""))
		ev := c.warningEvent(pod, "Pod", reason, message, "kubelet", 3)
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
	}
}
