package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ambiguousScenarios are failures whose best explanation kwatch states
// as "likely" and may get wrong: two plausible causes close in score, a
// change near the failure with a weak link, a shared node against an
// app regression, a config edit against an image bump, a slow
// dependency against a slow app. Each is labelled with the root a
// person would conclude from the whole story, not with what kwatch
// says; the calibration gate counts how often "likely" is right.
func ambiguousScenarios() []scenario {
	return []scenario{
		ambiguousInitSecretEdited(), ambiguousInitNoChange(),
		ambiguousCronSecretEdited(), ambiguousCronNoChange(),
		ambiguousOOMNearLimit(), ambiguousOOMHalfLimit(),
		ambiguousCeilingSlowDependency(), ambiguousCeilingDependencyFine(),
		ambiguousImageAndConfigImage(), ambiguousImageAndConfigConfig(),
		ambiguousRolloutOnePressuredNode(), ambiguousRolloutEveryNode(),
	}
}

// ambiguousInit builds the orders Deployment whose migration init
// container reads its password from Secret orders-db. When edited, the
// secret is changed by bob a few minutes before the init container
// starts to fail with an error that names no key.
func ambiguousInit(c *cluster, edited bool) {
	secret := configSecret(c, "shop", "orders-db", "password")
	c.list(c.node("n1", "zone-a"), secret)
	w := c.deployment("shop", "orders", "registry.example.com/orders:8", 1)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.InitContainers = []corev1.Container{{
			Name: "migrate", Image: "registry.example.com/migrate:8",
			Env: []corev1.EnvVar{configSecretEnv("DB_PASSWORD",
				"orders-db", "password")},
		}}
	})
	c.list(w.objects())
	c.after(time.Minute)
	if edited {
		change := last(c, secret)
		change.Data["password"] = []byte("rotated")
		editedBy(c, change, "bob")
		c.update(change)
		c.after(time.Minute)
	}
	message := "migrate: authentication failed for user orders"
	for restarts := int32(1); restarts <= 6; restarts++ {
		c.update(w.pod(0, "n1", initCrashLoop(message, restarts)))
		w.setReady(0)
		c.update(w.objects())
		c.after(45 * time.Second)
	}
}

// ambiguousInitSecretEdited: the migration fails on authentication
// a minute after bob rotated the password Secret it reads. The
// Secret is the probable root, but the failure could as well be the
// migration's own user; the story says the rotation was the change.
func ambiguousInitSecretEdited() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-init-secret-edited",
			Description: "An init container fails on authentication " +
				"minutes after the Secret it reads was edited.",
			Root: "secret/shop/orders-db", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) { ambiguousInit(c, true) },
	}
}

// ambiguousInitNoChange: the same failure with no edit anywhere; the
// migration itself is broken.
func ambiguousInitNoChange() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-init-no-change",
			Description: "An init container fails on authentication " +
				"and nothing it reads changed.",
			Root: "deployment/shop/orders", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1", "secret/shop/orders-db"},
		},
		build: func(c *cluster) { ambiguousInit(c, false) },
	}
}

// ambiguousCron builds the export CronJob that reads Secret export-creds
// and whose latest run failed; when edited the Secret was changed
// shortly before.
func ambiguousCron(c *cluster, edited bool) {
	secret := configSecret(c, "reports", "export-creds", "token")
	cron := scheduleCronJob(c, "reports", "nightly-export", "0 2 * * *")
	spec := &cron.Spec.JobTemplate.Spec.Template.Spec
	spec.Containers[0].Env = []corev1.EnvVar{configSecretEnv("API_TOKEN",
		"export-creds", "token")}
	c.list(c.node("n1", "zone-a"), secret, cron)
	c.after(30 * time.Minute)
	if edited {
		change := last(c, secret)
		change.Data["token"] = []byte("rotated")
		editedBy(c, change, "bob")
		c.update(change)
		c.after(2 * time.Hour)
	}
	failedAt := c.now
	job := jobOf(c, cron, "nightly-export-1", failedAt)
	job.Status.Failed = 4
	job.Status.Conditions = ambiguousJobFailed(failedAt)
	c.list(job)
	c.after(10 * time.Minute)
}

// ambiguousCronSecretEdited: the nightly export fails its only run two
// hours after its token Secret was rotated.
func ambiguousCronSecretEdited() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-cron-secret-edited",
			Description: "A CronJob's run fails two hours after the " +
				"Secret it reads was rotated.",
			Root: "secret/reports/export-creds", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) { ambiguousCron(c, true) },
	}
}

// ambiguousCronNoChange: the same failed run with nothing edited.
func ambiguousCronNoChange() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-cron-no-change",
			Description: "A CronJob's run fails and nothing it reads " +
				"was edited.",
			Root: "cronjob/reports/nightly-export", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) { ambiguousCron(c, false) },
	}
}

// ambiguousOOMNearLimit: the node is under memory pressure and its
// limits are overcommitted, but api was at 470Mi of its 512Mi limit:
// it simply needs more memory, whatever the node does.
func ambiguousOOMNearLimit() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-oom-near-limit",
			Description: "A container is OOM-killed at 92% of its limit " +
				"on an overcommitted node under memory pressure.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildNodeOOMWith(c, 470, "30Gi", true) },
	}
}

// ambiguousOOMHalfLimit: the same kill at half the limit: the node ran
// out of memory first.
func ambiguousOOMHalfLimit() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-oom-half-limit",
			Description: "A container is OOM-killed at half its limit " +
				"on an overcommitted node under memory pressure.",
			Root: "node//n1", Tier: "notify", MaxMessages: 3,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildNodeOOMWith(c, 260, "30Gi", true) },
	}
}

// ceilingWithDependency is autoscalerCeiling.build for a worker whose
// containers read their database address, with kwatch's probe of that
// database answering failure (empty when it answers).
func ceilingWithDependency(c *cluster, failure string) {
	a := autoscalerCeiling{
		namespace: "orders", name: "worker", from: 4, max: 8, target: 60,
		nodes: []string{"w7", "w8"},
		probe: "Readiness probe failed: Get \"http://10.244.3.7:9000/" +
			"ready\": context deadline exceeded",
	}
	for _, node := range a.nodes {
		c.list(c.node(node, "zone-a"))
	}
	w := c.deployment(a.namespace, a.name,
		"registry.example.com/"+a.name+":3", a.from)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{Name: "DATABASE_URL",
			Value: "postgres://orders@" + ordersDatabase + "/orders"}}
	})
	c.list(w.objects())
	for i := range int(a.from) {
		c.list(w.pod(i, a.node(i)))
	}
	db := inventory.CoreID(kube.KindExternalEndpoint, "", ordersDatabase)
	c.probe(db, "")
	c.list(a.hpa(c, a.from, a.from, a.target-10, false))
	c.after(3 * time.Minute)
	setReplicas(w, a.max)
	w.setReady(a.from)
	c.update(w.objects())
	c.update(a.hpa(c, a.from, a.max, 130, false))
	for i := int(a.from); i < int(a.max); i++ {
		c.create(w.pod(i, a.node(i), startedNow))
	}
	w.setReady(a.max)
	c.update(w.objects())
	c.after(90 * time.Second)
	c.update(a.hpa(c, a.max, a.max, 240, true))
	for round := int32(1); round <= 5; round++ {
		c.probe(db, failure)
		for i := range int(a.from) {
			pod := w.pod(i, a.node(i), notReady)
			c.update(pod)
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", a.probe,
				"kubelet", round*2))
		}
		w.setReady(a.max - a.from)
		c.update(w.objects())
		c.after(time.Minute)
	}
}

// ambiguousCeilingSlowDependency: the HPA is at its ceiling and the
// replicas time out, but the database they all wait on stopped
// answering kwatch's probe too. More replicas would not help: the
// database is the root, the ceiling a consequence of the retries.
func ambiguousCeilingSlowDependency() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-ceiling-slow-dependency",
			Description: "An HPA at its maximum while the replicas time " +
				"out and the database they call times out kwatch's " +
				"probe as well.",
			Root: "external-endpoint//" + ordersDatabase, Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//w7", "node//w8"},
		},
		build: func(c *cluster) {
			ceilingWithDependency(c, "dial tcp: i/o timeout")
		},
	}
}

// ambiguousCeilingDependencyFine: the same ceiling and timeouts with the
// database answering every probe: the limit is too low.
func ambiguousCeilingDependencyFine() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-ceiling-dependency-fine",
			Description: "An HPA at its maximum while the replicas time " +
				"out and the database they call answers kwatch's probe.",
			Root: "horizontalpodautoscaler/orders/worker", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//w7", "node//w8",
				"external-endpoint//" + ordersDatabase},
		},
		build: func(c *cluster) { ceilingWithDependency(c, "") },
	}
}

// ambiguousChange lists the checkout Deployment on two nodes with its
// ConfigMap, then bumps the image and edits the ConfigMap thirty seconds
// apart. Both replicas crash with trace.
func ambiguousChange(c *cluster, trace string) {
	cm := configMap(c, "shop", "checkout-config",
		map[string]string{"currency": "EGP", "tax": "14"})
	w := c.deployment("shop", "checkout",
		"registry.example.com/checkout:2.3", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cm.Name}}}}
	})
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(10 * time.Minute)
	edited := last(c, cm)
	edited.Data["tax"] = "0.14"
	editedBy(c, edited, "bob")
	c.update(edited)
	c.after(30 * time.Second)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/checkout:2.4"
	})
	c.update(w.deployment)
	c.create(rs)
	c.create(w.pod(0, "n1", startedNow, notReady))
	c.after(40 * time.Second)
	for _, restarts := range []int32{2, 4, 6} {
		for i := range 2 {
			c.update(w.pod(i, "n"+itoa(i+1), startedNow,
				crashLoop(1, "Error", trace, restarts)))
		}
		w.setReady(0)
		c.update(w.objects())
		c.after(90 * time.Second)
	}
}

// ambiguousImageAndConfigImage: the image bump and the ConfigMap edit
// land together; the panic is in code that only exists in 2.4.
func ambiguousImageAndConfigImage() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-image-and-config-image",
			Description: "A new image and an edited ConfigMap arrive " +
				"within a minute and the pods panic in code new to " +
				"the image.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			ambiguousChange(c, "panic: runtime error: index out of "+
				"range at checkout/v2/pricing.go:88 (new in 2.4)")
		},
	}
}

// ambiguousImageAndConfigConfig: the same pair; the panic is in the
// config loader on the value bob changed.
func ambiguousImageAndConfigConfig() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-image-and-config-config",
			Description: "A new image and an edited ConfigMap arrive " +
				"within a minute and the pods panic parsing their " +
				"settings.",
			Root: "configmap/shop/checkout-config", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			ambiguousChange(c, "panic: strconv.ParseInt: parsing "+
				"\"0.14\": invalid syntax at config.Load")
		},
	}
}
