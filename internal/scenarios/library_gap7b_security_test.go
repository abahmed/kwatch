package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// gap7bScenarios are security and multi-tenancy plumbing: encryption,
// authorization, identities, RBAC, network policy and admission rules.
func gap7bScenarios() []scenario {
	return []scenario{
		gap7bKMSDown(), gap7bOldKeyDropped(), gap7bAuthorizerDown(),
		gap7bServiceAccountRecreated(), gap7bLeasesForbidden(),
		gap7bClusterRoleReverted(), gap7bPolicyBlocksWebhook(),
		gap7bPolicyBlocksMetrics(), gap7bCSRNotIssued(),
		gap7bPolicyExpressionError(), gap7bDropAllCapabilities(),
		gap7bRunAsUserNoPasswd(),
	}
}

// gap7bConfigError plays new pods of fleet in ContainerConfigError with
// message for rounds minutes.
func gap7bConfigError(
	c *cluster, fleet []*workload, message string, rounds int32,
) {
	for _, w := range fleet {
		*w.deployment.Spec.Replicas = 2
		w.setReady(1)
		c.update(w.objects())
	}
	for round := int32(1); round <= rounds; round++ {
		for _, w := range fleet {
			pod := w.pod(1, "n2", startedNow, gapPending(waiting(
				"CreateContainerConfigError", message)))
			c.update(pod)
			gapKubelet(c, pod, "Failed", "Error: "+message, round)
		}
		c.after(time.Minute)
	}
}

// gap7bKMSDown: the encryption provider stops answering; every read of a
// Secret fails, so new pods that reference one cannot start in three
// namespaces.
func gap7bKMSDown() scenario {
	return scenario{
		expect: g2Exp("encryption-provider-down-secret-reads-fail",
			"The encryption provider stops answering; new pods that "+
				"reference a Secret cannot start in three namespaces "+
				"because every Secret read fails.",
			"unknown", "notify", "node//n1", "deployment/shop/web",
			"deployment/billing/invoices", "deployment/media/transcode").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, []string{"n1"})
			c.after(10 * time.Minute)
			gap7bConfigError(c, fleet, "couldn't get secret creds: Internal "+
				"error occurred: failed to decrypt data: rpc error: code "+
				"= DeadlineExceeded desc = context deadline exceeded", 6)
		},
	}
}

// gap7bOldKeyDropped: the old encryption key was dropped from the
// configuration; Secrets written before the change cannot be read, newer
// ones can.
func gap7bOldKeyDropped() scenario {
	return scenario{
		expect: g2Exp("encryption-key-dropped-old-secrets-unreadable",
			"After a key was dropped from the encryption configuration, "+
				"pods that reference a Secret written before it cannot "+
				"start while pods with newer Secrets do.",
			"unknown", "notify", "node//n1", "deployment/shop/orders",
			"deployment/shop/payments").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/orders", "shop/payments"},
				[]string{"n1"})
			c.after(10 * time.Minute)
			gap7bConfigError(c, fleet[:1], "couldn't get secret orders-db: "+
				"Internal error occurred: failed to decrypt data: no "+
				"matching key found for the stored data", 6)
			fleet[1].setReady(1)
		},
	}
}

// gap7bAuthorizerDown: the authorization webhook stops answering; the
// controllers of three namespaces are refused at the same minute with no
// RBAC change.
func gap7bAuthorizerDown() scenario {
	return scenario{
		expect: g2Exp("authorization-webhook-down-forbidden-everywhere",
			"Controllers in three unrelated namespaces are refused "+
				"\"forbidden\" in the same minute while no Role or "+
				"binding changed.",
			"unknown", "notify", "node//n1", "deployment/shop/operator",
			"deployment/billing/operator", "deployment/media/operator").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/operator",
				"billing/operator", "media/operator"}, []string{"n1"})
			c.after(10 * time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for _, w := range fleet {
					c.update(w.pod(0, "n1", crashLoop(1, "Error", "pods "+
						"is forbidden: User \"system:serviceaccount:"+
						"operator\" cannot list resource \"pods\" in API "+
						"group \"\": Internal error occurred: authorization "+
						"webhook: context deadline exceeded", restarts)))
					w.setReady(0)
					c.update(w.objects())
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap7bServiceAccountRecreated: a GitOps sync deletes and creates the
// ServiceAccount; the tokens of the running pods name the old identity
// and are rejected.
func gap7bServiceAccountRecreated() scenario {
	return scenario{
		expect: g2Exp("service-account-recreated-running-pods-unauthorized",
			"A ServiceAccount is deleted and created again with the "+
				"same name; the running pods' tokens are rejected and "+
				"they exit with 401 Unauthorized.",
			"serviceaccount/shop/api", "notify", "node//n1",
			"deployment/shop/api").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			account := &corev1.ServiceAccount{ObjectMeta: c.meta("shop", "api")}
			c.list(account)
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			// The pods run as the account the sync recreates.
			w.deployment.Spec.Template.Spec.ServiceAccountName = "api"
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			c.remove(account)
			fresh := &corev1.ServiceAccount{ObjectMeta: c.meta("shop", "api")}
			fresh.UID = "uid-api-recreated"
			fresh.CreationTimestamp = metav1.NewTime(c.now)
			c.create(fresh)
			c.after(time.Minute)
			message := "error: You must be logged in to the server " +
				"(Unauthorized)"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), w.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap7bLeasesForbidden: an operator upgrade needs leader election; its
// namespaced Role lacks the verbs on leases and it exits at start.
func gap7bLeasesForbidden() scenario {
	return scenario{
		expect: g2Exp("operator-role-lacks-lease-permission",
			"An operator upgrade starts leader election but its Role "+
				"lacks access to leases; it crash-loops on \"leases is "+
				"forbidden\".",
			"role/shop/operator-role", "notify", "node//n1",
			"deployment/shop/operator").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(rbacObject(c, "Role", "shop", "operator-role",
				map[string]any{"rules": []any{map[string]any{
					"apiGroups": []any{""},
					"resources": []any{"configmaps"},
					"verbs":     []any{"get", "list", "watch"}}}}))
			w := c.deployment("shop", "operator",
				"registry.example.com/operator:2.0", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(10 * time.Minute)
			message := "error retrieving resource lock shop/operator-" +
				"lock: leases.coordination.k8s.io \"operator-lock\" is " +
				"forbidden: User \"system:serviceaccount:shop:operator\" " +
				"cannot get resource \"leases\" in API group " +
				"\"coordination.k8s.io\" in the namespace \"shop\""
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap7bClusterRoleReverted: an operator adds a rule to an aggregated
// ClusterRole; the next reconcile reverts it and the app that relied on
// it is refused.
func gap7bClusterRoleReverted() scenario {
	return scenario{
		expect: g2Exp("aggregated-cluster-role-edit-reverted",
			"A rule added to an aggregated ClusterRole is reverted at "+
				"the next reconcile; the app that relied on it starts "+
				"to be refused \"forbidden\".",
			"clusterrole//view-extended", "notify", "node//n1",
			"deployment/shop/dashboard").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			role := func(resources ...any) *unstructured.Unstructured {
				return rbacObject(c, "ClusterRole", "", "view-extended",
					map[string]any{"rules": []any{map[string]any{
						"apiGroups": []any{""}, "resources": resources,
						"verbs": []any{"get", "list"}}}})
			}
			c.list(role("pods"))
			w := c.deployment("shop", "dashboard",
				"registry.example.com/dashboard:5", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(5 * time.Minute)
			c.update(role("pods", "nodes"))
			c.after(30 * time.Minute)
			c.update(role("pods"))
			message := "nodes is forbidden: User \"system:serviceaccount:" +
				"shop:dashboard\" cannot list resource \"nodes\" in API " +
				"group \"\" at the cluster scope"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap7bDenyAll is a default-deny ingress policy of namespace policy-system.
func gap7bDenyAll(c *cluster) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: clusterMeta(c, "policy-system", "default-deny",
			"netpol"),
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress}}}
}

// gap7bPolicyBlocksWebhook: a default-deny policy is added to the namespace
// of a fail-closed webhook's backend; the API server's calls time out
// although the backend's endpoints are ready.
func gap7bPolicyBlocksWebhook() scenario {
	return scenario{
		expect: g2Exp("networkpolicy-blocks-api-server-to-webhook-backend",
			"A default-deny policy added to a webhook backend's "+
				"namespace makes its calls from the API server time "+
				"out; pod creation fails in two namespaces while the "+
				"backend's endpoints are ready.",
			"networkpolicy/policy-system/default-deny", "page",
			"validatingwebhookconfiguration//image-guard",
			"deployment/policy-system/guard", "node//n1").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			fleet := gap6Webhook(c, "image-guard")
			c.after(10 * time.Minute)
			c.create(gap7bDenyAll(c))
			c.after(2 * time.Minute)
			gap6Rollouts(c, fleet, "Internal error occurred: failed "+
				"calling webhook \"image-guard.policy.example.com\": "+
				"failed to call webhook: Post \"https://guard.policy-"+
				"system.svc:443/validate?timeout=10s\": context "+
				"deadline exceeded", 6)
		},
	}
}

// gap7bPolicyBlocksMetrics: a policy isolates the metrics server's pods;
// the APIService reports Available=False and HPAs stop scaling.
func gap7bPolicyBlocksMetrics() scenario {
	return scenario{
		expect: g2Exp("networkpolicy-isolates-metrics-server",
			"A policy isolates the metrics server's pods; the metrics "+
				"APIService turns Available=False on a discovery "+
				"timeout and the HPA cannot read metrics.",
			"networkpolicy/kube-system/isolate-metrics", "notify",
			"deployment/kube-system/metrics-server",
			"apiservice//v1beta1.metrics.k8s.io", "deployment/shop/web").
			tail(20 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			server := metricsServer(c)
			c.list(server.objects())
			c.list(server.pod(0, "n1"), server.pod(1, "n1"))
			c.list(clusterService(c, "kube-system", "metrics-server", 10250))
			c.list(metricsSlice(c, server, true))
			c.list(clusterAPIService(c, "True", "Passed",
				"all checks passed"))
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.list(clusterHPA(c, "shop", "web", "True", "ValidMetricFound",
				""))
			c.after(5 * time.Minute)
			c.create(&networkingv1.NetworkPolicy{
				ObjectMeta: clusterMeta(c, "kube-system", "isolate-metrics",
					"netpol"),
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{MatchLabels: server.
						deployment.Spec.Template.Labels},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress}}})
			c.after(2 * time.Minute)
			c.update(clusterAPIService(c, "False", "FailedDiscoveryCheck",
				"failing or missing response from https://10.244.1.7:"+
					"10250/apis/metrics.k8s.io/v1beta1: Get \"https://"+
					"10.244.1.7:10250/apis/metrics.k8s.io/v1beta1\": "+
					"context deadline exceeded"))
			message := "unable to fetch metrics from resource metrics API: " +
				"the server is currently unable to handle the request"
			for m := int32(1); m <= 6; m++ {
				hpa := clusterHPA(c, "shop", "web", "False",
					"FailedGetResourceMetric", message)
				c.update(hpa)
				g2Event(c, hpa, "HorizontalPodAutoscaler",
					"FailedGetResourceMetric", message,
					"horizontal-pod-autoscaler", m)
				c.after(time.Minute)
			}
		},
	}
}
