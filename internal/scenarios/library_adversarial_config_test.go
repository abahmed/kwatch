package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// advConfigScenarios are adversarial stories about configuration,
// access and custom resources: edits and deletions that sit beside a
// failure without causing it, and failures whose cause hides behind a
// controller.
func advConfigScenarios() []scenario {
	return []scenario{
		advBadConfigStuckDrain(), advBadConfigCleanDrain(),
		advRBACTwoBindings(), advForbiddenPartnerAPI(),
		advSimilarNameConfigMap(), advTwoCauses(),
		advConfigMapRecreatedBlip(),
	}
}

const advTaxCrash = "config: TAX_RATE: strconv.ParseFloat: parsing " +
	"\"n/a\": invalid syntax"

func advEnvFromConfigMap(name string) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) {
		spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name}}}}
	}
}

// advRestart rolls the Deployment as kubectl rollout restart does: the
// pod template gains a restartedAt annotation and nothing else changes.
func advRestart(c *cluster, w *workload) {
	template := &w.deployment.Spec.Template
	template.Annotations = map[string]string{
		"kubectl.kubernetes.io/restartedAt": c.now.Format(time.RFC3339)}
	rs := w.rollout(func(*corev1.PodSpec) {})
	c.update(w.deployment)
	c.create(rs)
}

// advBadConfigSetup lists the shared cast of the two drain scenarios and
// returns the invoicer, the ledger and the ConfigMap. The invoicer
// reads its ConfigMap as environment, so only new pods see an edit.
func advBadConfigSetup(
	c *cluster, ledgerReplicas int32,
) (inv, ledger *workload, cm *corev1.ConfigMap, n2 *corev1.Node) {
	n2 = c.node("n2", "zone-a")
	c.list(c.node("n1", "zone-a"), n2, c.node("n3", "zone-a"))
	cm = configMap(c, "billing", "invoicer-config",
		map[string]string{"TAX_RATE": "0.14", "CURRENCY": "EGP"})
	inv = c.deployment("billing", "invoicer",
		"registry.example.com/invoicer:3.2", 2)
	configTemplate(inv, advEnvFromConfigMap(cm.Name))
	ledger = c.deployment("shop", "ledger",
		"registry.example.com/ledger:9", ledgerReplicas)
	c.list(cm)
	c.list(inv.objects())
	c.list(inv.pod(0, "n1"), inv.pod(1, "n3"))
	c.list(ledger.objects())
	return inv, ledger, cm, n2
}

func advCordon(c *cluster, node *corev1.Node) {
	cordoned := last(c, node)
	cordoned.Spec.Unschedulable = true
	cordoned.Spec.Taints = []corev1.Taint{{
		Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
	}}
	c.update(cordoned)
}

func advBreakConfig(c *cluster, inv *workload, cm *corev1.ConfigMap) {
	edited := last(c, cm)
	edited.Data["TAX_RATE"] = "n/a"
	c.update(edited)
	c.after(30 * time.Second)
	advRestart(c, inv)
	c.create(inv.pod(0, "n1", startedNow, notReady))
}

// advBadConfigStuckDrain: a ConfigMap edit followed by a restart makes
// the new invoicer pod crash, while the old pods keep serving. At the
// same time n2 is drained for an upgrade and a budget stops the drain.
// Two unrelated problems, two incidents.
func advBadConfigStuckDrain() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-bad-config-and-stuck-drain",
			Description: "A bad ConfigMap value breaks a restarted " +
				"Deployment while, unrelated, a drain of another node " +
				"is blocked by a PodDisruptionBudget.",
			Root:        "configmap/billing/invoicer-config",
			OtherRoots:  []string{"poddisruptionbudget/shop/ledger"},
			Tier:        "notify",
			MaxMessages: 5,
			MustNotBlame: []string{"node//n2", "deployment/billing/invoicer",
				"deployment/shop/ledger", "node//n1"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			inv, ledger, cm, n2 := advBadConfigSetup(c, 1)
			c.list(ledger.pod(0, "n2"))
			c.list(lifecycleBudget(c, ledger, 0))
			c.after(2 * time.Minute)
			advBreakConfig(c, inv, cm)
			advCordon(c, n2)
			for minute := int32(1); minute <= 13; minute++ {
				c.after(time.Minute)
				c.update(inv.pod(0, "n1", startedNow, crashLoop(1, "Error",
					advTaxCrash, minute+2)))
				c.update(lifecycleBudget(c, ledger, 0))
			}
		},
	}
}

// advBadConfigCleanDrain: the same broken restart while n2 is drained
// cleanly: the ledger has two replicas, the budget allows one
// disruption and the evicted pod is replaced in seconds. One problem.
func advBadConfigCleanDrain() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-bad-config-clean-drain",
			Description: "A bad ConfigMap value breaks a restarted " +
				"Deployment while a drain of another node completes " +
				"within its budget.",
			Root: "configmap/billing/invoicer-config", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n2", "deployment/billing/invoicer",
				"deployment/shop/ledger", "node//n1"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			inv, ledger, cm, n2 := advBadConfigSetup(c, 2)
			c.list(ledger.pod(0, "n2"), ledger.pod(1, "n3"))
			c.list(lifecycleBudget(c, ledger, 1))
			c.after(2 * time.Minute)
			advBreakConfig(c, inv, cm)
			advCordon(c, n2)
			c.after(20 * time.Second)
			c.remove(ledger.pod(0, "n2"))
			ledger.setReady(1)
			c.update(ledger.objects())
			c.create(ledger.pod(2, "n1", startedNow, notReady))
			c.after(15 * time.Second)
			c.update(ledger.pod(2, "n1", startedNow))
			ledger.setReady(2)
			c.update(ledger.objects())
			for minute := int32(1); minute <= 8; minute++ {
				c.after(time.Minute)
				c.update(inv.pod(0, "n1", startedNow, crashLoop(1, "Error",
					advTaxCrash, minute+2)))
			}
		},
	}
}

// advTwoCauses: a ConfigMap and a Secret that checkout reads at start
// are both changed within a minute, then checkout is restarted and
// crash-loops with an error that names neither. Both are equally
// plausible. The honest answer is not one of them stated as fact, and
// it must not change from tick to tick as restarts pile up.
func advTwoCauses() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-two-causes-stable",
			Description: "A ConfigMap edit and a Secret rotation land " +
				"together before a restart; the crash names neither.",
			Root: "unknown", Tier: "notify", MaxMessages: 2,
			// Two equally plausible causes: a hedge that names both is
			// accepted, one of them stated alone is not.
			Ambiguous: true,
			MustNotBlame: []string{"node//n1", "node//n2",
				"deployment/shop/checkout"},
			Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			cm := configMap(c, "shop", "checkout-flags",
				map[string]string{"FLAG_NEW_TAX": "off"})
			secret := configSecret(c, "shop", "checkout-keys",
				"signing-key")
			w := c.deployment("shop", "checkout",
				"registry.example.com/checkout:8.4", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				advEnvFromConfigMap(cm.Name)(spec)
				spec.Containers[0].Env = []corev1.EnvVar{
					configSecretEnv("SIGNING_KEY", secret.Name,
						"signing-key")}
			})
			c.list(cm, secret, w.deployment, w.replicaSet)
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(2 * time.Minute)
			editedMap := last(c, cm)
			editedMap.Data["FLAG_NEW_TAX"] = "on"
			c.update(editedMap)
			c.after(40 * time.Second)
			rotated := last(c, secret)
			rotated.Data["signing-key"] = []byte("rotated-key")
			c.update(rotated)
			c.after(20 * time.Second)
			advRestart(c, w)
			crash := "fatal: initialization failed (exit 1)"
			for restarts := int32(1); restarts <= 10; restarts++ {
				c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
					crash, restarts)), w.pod(1, "n2", startedNow,
					crashLoop(1, "Error", crash, restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(90 * time.Second)
			}
		},
	}
}

// advConfigMapRecreatedBlip: a GitOps resync deletes and recreates a
// ConfigMap within fifteen seconds. Two replicas the autoscaler added in
// that window report CreateContainerConfigError for forty seconds and
// then run. Nothing lasted long enough to be an outage.
func advConfigMapRecreatedBlip() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-configmap-recreated-blip",
			Description: "A ConfigMap is deleted and recreated by a " +
				"resync; pods created in between fail for forty seconds " +
				"and recover.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			cm := configMap(c, "web", "web-config",
				map[string]string{"LEVEL": "info"})
			w := c.deployment("web", "site", "registry.example.com/site:2", 3)
			configTemplate(w, advEnvFromConfigMap(cm.Name))
			c.list(cm, w.deployment, w.replicaSet)
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n1"))
			c.after(2 * time.Minute)
			c.remove(cm)
			c.after(5 * time.Second)
			setReplicas(w, 5)
			w.setReady(3)
			c.update(w.objects())
			message := "configmap \"" + cm.Name + "\" not found"
			stuck := []*corev1.Pod{
				w.pod(3, "n1", startedNow, waiting(
					"CreateContainerConfigError", message)),
				w.pod(4, "n2", startedNow, waiting(
					"CreateContainerConfigError", message))}
			c.create(stuck[0], stuck[1])
			configWarn(c, w, 3, 5, "Failed", "Error: "+message)
			c.after(10 * time.Second)
			c.create(configMap(c, "web", "web-config",
				map[string]string{"LEVEL": "info"}))
			c.after(25 * time.Second)
			c.update(w.pod(3, "n1", startedNow), w.pod(4, "n2", startedNow))
			w.setReady(5)
			c.update(w.objects())
		},
	}
}
