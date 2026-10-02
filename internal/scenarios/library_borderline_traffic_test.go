package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// borderlineTrafficScenarios are more ambiguous labelled cases, about
// routes, policies and helper containers. See borderlineScenarios.
func borderlineTrafficScenarios() []scenario {
	return []scenario{
		borderlineRouteAndGatewayEdit(), borderlineTwoPolicies(),
		borderlineInitAfterRotation(),
	}
}

// borderlineRouteAndGatewayEdit: the Gateway is edited (its listener
// gets a new certificate reference) and thirty seconds later the web
// HTTPRoute is edited to attach to a listener the Gateway does not
// have. The route edit is the root; the Gateway edit is the plausible
// alternative.
func borderlineRouteAndGatewayEdit() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-route-and-gateway-edit",
			Description: "A Gateway edit and an HTTPRoute edit land " +
				"thirty seconds apart; the route names a listener the " +
				"Gateway does not have.",
			Root:        "httproute.gateway.networking.k8s.io/shop/web",
			Tier:        "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"service/shop/web",
				"deployment/shop/web"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			gateway := trafficGateway(c)
			c.list(gateway, trafficRoute(c, 1, "https", "True",
				"Accepted", ""))
			c.after(time.Minute)
			edited := gateway.DeepCopy()
			edited.SetGeneration(2)
			edited.Object["spec"].(map[string]any)["listeners"] = []any{
				map[string]any{"name": "https", "port": int64(443),
					"protocol": "HTTPS", "tls": map[string]any{
						"certificateRefs": []any{map[string]any{
							"name": c.n("web-tls-2026")}}}}}
			c.update(edited)
			c.after(30 * time.Second)
			c.update(trafficRoute(c, 2, "web-https", "False",
				"NoMatchingParent", "No listener named web-https"))
			c.after(5 * time.Minute)
		},
	}
}

// borderlineTwoPolicies: a minute apart, the payments team adds a policy
// allowing the api to reach the cache and another that denies all of
// the api's egress except DNS. The api times out on its database. The
// deny policy is the root; the allow policy is the plausible
// alternative.
func borderlineTwoPolicies() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-two-policies",
			Description: "Two egress NetworkPolicies for the same pods " +
				"land a minute apart, one allowing and one denying; the " +
				"pods time out reaching their database.",
			Root: "networkpolicy/payments/restrict-egress", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"cluster-dns//cluster-dns"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			api := c.deployment("payments", "api",
				"registry.example.com/pay:6", 2)
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(time.Minute)
			c.create(borderlineAllowCache(c, api))
			c.after(time.Minute)
			c.create(clusterEgressPolicy(c, api))
			c.after(20 * time.Second)
			message := "FATAL: failed to connect to `host=10.0.3.4 " +
				"user=payments database=payments`: dial error (dial tcp " +
				"10.0.3.4:5432: i/o timeout)"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(api.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), api.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				api.setReady(0)
				c.update(api.objects())
				c.after(45 * time.Second)
			}
		},
	}
}

// borderlineAllowCache allows the api's egress to the cache port.
func borderlineAllowCache(
	c *cluster, w *workload,
) *networkingv1.NetworkPolicy {
	tcp := corev1.ProtocolTCP
	redis := intstr.FromInt32(6379)
	return &networkingv1.NetworkPolicy{
		ObjectMeta: clusterMeta(c, "payments", "allow-cache", "netpol"),
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: w.deployment.Spec.Template.Labels,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &tcp, Port: &redis},
				},
			}},
		},
	}
}

// borderlineInitAfterRotation: the orders database password in Secret
// orders-db is rotated, but the database still has the old one. Two
// minutes later a rollout restarts the pods, and their migration init
// container fails to log in. The rotation is the root; the rollout,
// which is what restarted the pods, is the plausible alternative.
func borderlineInitAfterRotation() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-init-after-rotation",
			Description: "A Secret rotation and a rollout land two " +
				"minutes apart; the new pods' init container fails " +
				"password authentication.",
			Root: "secret/shop/orders-db", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			secret := configSecret(c, "shop", "orders-db", "password")
			w := c.deployment("shop", "orders",
				"registry.example.com/orders:8", 1)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.InitContainers = []corev1.Container{{
					Name: "migrate", Image: "registry.example.com/migrate:8",
					Env: []corev1.EnvVar{configSecretEnv("PGPASSWORD",
						secret.Name, "password")},
				}}
			})
			c.list(secret)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(time.Minute)
			rotated := last(c, secret)
			rotated.Data["password"] = []byte("rotated-2026-09")
			c.update(rotated)
			c.after(2 * time.Minute)
			old := last(c, w.pod(0, ""))
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/orders:8.1"
			})
			c.update(w.deployment)
			c.create(rs)
			c.remove(old)
			message := "psql: error: connection to server at \"10.0.3.9\", " +
				"port 5432 failed: FATAL:  password authentication " +
				"failed for user \"orders\""
			rolledOut := c.now
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(w.pod(0, "n1", createdAt(rolledOut),
					initCrashLoop(message, restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(45 * time.Second)
			}
		},
	}
}
