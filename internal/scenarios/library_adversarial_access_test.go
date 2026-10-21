package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func advForbiddenLine(c *cluster, account string) string {
	return "level=fatal msg=\"configmaps \\\"api-settings\\\" is " +
		"forbidden: User \\\"system:serviceaccount:" + c.n("shop") + ":" +
		account + "\\\" cannot get resource \\\"configmaps\\\" in API " +
		"group \\\"\\\" in the namespace \\\"" + c.n("shop") + "\\\"\""
}

// advLogCrash crash-loops both replicas of w with an empty termination
// message and records first error line of the previous log, as the
// crash-log round does.
func advLogCrash(c *cluster, w *workload, line string, rounds int32) {
	for restarts := int32(1); restarts <= rounds; restarts++ {
		for i, node := range []string{"n1", "n2"} {
			pod := w.pod(i, node, crashLoop(1, "Error", "", restarts))
			c.update(pod)
			id := kube.ContainerID(pod.Namespace, pod.Name,
				pod.Spec.Containers[0].Name)
			c.emit(kube.CrashLogObservation(id, c.now,
				[]string{"info: starting", line}))
		}
		w.setReady(0)
		c.update(w.objects())
		c.after(45 * time.Second)
	}
}

// advRBACTwoBindings: GitOps prunes two RoleBindings of shop at once,
// the one that lets api read ConfigMaps and the one web used to. Three
// minutes later api restarts and its log says the api account may not
// read the ConfigMap; web keeps running. Only the api binding is the
// root; the web binding was removed too but nothing needed it.
func advRBACTwoBindings() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-rbac-two-bindings",
			Description: "Two RoleBindings are deleted together; only the " +
				"one for the failing ServiceAccount matters, and the " +
				"error is only in the previous log.",
			Root: "rolebinding/shop/api-config-reader", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"rolebinding/shop/web-config-reader",
				"deployment/shop/api", "deployment/shop/web", "node//n1",
				"node//n2"},
		},
		build: func(c *cluster) {
			api, _ := accessFleet(c)
			apiBinding := accessRoleBinding(c, "api-config-reader", "api")
			webBinding := accessRoleBinding(c, "web-config-reader", "web")
			c.list(apiBinding, webBinding)
			c.after(time.Minute)
			c.remove(apiBinding, webBinding)
			c.after(3 * time.Minute)
			advLogCrash(c, api, advForbiddenLine(c, "api"), 5)
		},
	}
}

// advForbiddenPartnerAPI: api crash-loops because a partner's HTTP API
// answers 403 Forbidden. A RoleBinding of another account in the same
// namespace was deleted a minute earlier. The word "forbidden" and the
// RBAC change are a coincidence: no Kubernetes request was refused.
func advForbiddenPartnerAPI() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-forbidden-partner-api",
			Description: "An app crashes on an HTTP 403 Forbidden from a " +
				"partner API while an unrelated RoleBinding of the " +
				"namespace was just deleted.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"rolebinding/shop/metrics-reader",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			api, _ := accessFleet(c)
			binding := accessRoleBinding(c, "metrics-reader", "prometheus")
			c.list(binding)
			c.after(time.Minute)
			c.remove(binding)
			c.after(40 * time.Second)
			line := "level=fatal msg=\"POST https://api.partner.example/" +
				"v2/orders: 403 Forbidden: {\\\"error\\\":\\\"forbidden: " +
				"merchant key is not enabled for this endpoint\\\"}\""
			advLogCrash(c, api, line, 5)
		},
	}
}

// advSimilarNameConfigMap: payments crashes on a nil dereference. The
// ConfigMap payments-config was edited twenty seconds earlier, but it
// belongs to payments-batch; payments reads payments-settings. A name
// that starts the same is not a dependency.
func advSimilarNameConfigMap() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-similar-name-configmap",
			Description: "An app crashes on a nil dereference right " +
				"after a ConfigMap with a similar name, used by another " +
				"Deployment, was edited.",
			Root: "deployment/payments/payments", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"configmap/payments/payments-config",
				"configmap/payments/payments-settings",
				"deployment/payments/payments-batch"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			settings := configMap(c, "payments", "payments-settings",
				map[string]string{"MODE": "live"})
			batchConfig := configMap(c, "payments", "payments-config",
				map[string]string{"BATCH_SIZE": "500"})
			app := c.deployment("payments", "payments",
				"registry.example.com/payments:6.1", 2)
			configTemplate(app, advEnvFromConfigMap(settings.Name))
			batch := c.deployment("payments", "payments-batch",
				"registry.example.com/payments-batch:6.1", 1)
			configTemplate(batch, advEnvFromConfigMap(batchConfig.Name))
			c.list(settings, batchConfig)
			c.list(app.deployment, app.replicaSet, batch.deployment, batch.replicaSet)
			c.list(app.pod(0, "n1"), app.pod(1, "n2"), batch.pod(0, "n1"))
			c.after(2 * time.Minute)
			edited := last(c, batchConfig)
			edited.Data["BATCH_SIZE"] = "5000"
			c.update(edited)
			c.after(20 * time.Second)
			crash := "panic: runtime error: invalid memory address or " +
				"nil pointer dereference\n[signal SIGSEGV] " +
				"payments/ledger.(*Entry).Post"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(app.pod(0, "n1", crashLoop(2, "Error", crash,
					restarts)), app.pod(1, "n2", crashLoop(2, "Error",
					crash, restarts)))
				app.setReady(0)
				c.update(app.objects())
				c.after(45 * time.Second)
			}
		},
	}
}
