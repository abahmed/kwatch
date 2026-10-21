package scenarios

import (
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rcaEvidenceAccessScenarios are policy and permission objects that
// appear near a crash they cannot explain.
func rcaEvidenceAccessScenarios() []scenario {
	return []scenario{
		policyCreatedBeforeCodeCrash(), roleCreatedBeforeForbiddenCrash(),
		bindingOfAnotherAccountRemoved(),
	}
}

// policyCreatedBeforeCodeCrash: a NetworkPolicy that selects orders is
// created; four minutes later orders panics on a code defect. A policy
// restricts connections: a crash with no connection error is not its.
func policyCreatedBeforeCodeCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "policy-created-before-code-crash",
			Description: "A NetworkPolicy is created for an app; four " +
				"minutes later the app panics on a code defect.",
			Root: "deployment/shop/orders", Tier: "notify",
			MaxMessages:  4,
			MustNotBlame: []string{"networkpolicy/shop/default-deny"},
		},
		build: func(c *cluster) {
			w := c.deployment("shop", "orders",
				"registry.example.com/orders:4.1", 2)
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(5 * time.Minute)
			c.create(&networkingv1.NetworkPolicy{
				ObjectMeta: c.meta("shop", "default-deny"),
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
						"app": w.deployment.Name}},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress}}})
			c.after(4 * time.Minute)
			for round := int32(1); round <= 3; round++ {
				for i := 0; i < 2; i++ {
					c.update(w.pod(i, "n"+itoa(i+1),
						crashLoop(2, "Error", rcaEvidencePanic, round*3)))
				}
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// roleCreatedBeforeForbiddenCrash: an unrelated Role is created; api
// then crashes with a forbidden error. Creating a Role grants, it never
// takes a permission away, so it cannot be why a request was refused.
func roleCreatedBeforeForbiddenCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "role-created-before-forbidden-crash",
			Description: "An unrelated Role is created; the api crashes " +
				"with a forbidden error, a permission it never had.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 4,
			MustNotBlame: []string{"role/shop/metrics-reader"},
		},
		build: func(c *cluster) {
			api, _ := accessFleet(c)
			c.after(time.Minute)
			c.create(accessRole(c, "metrics-reader"))
			c.after(20 * time.Second)
			accessCrash(c, api, `panic: configmaps is forbidden: `+
				`User "system:serviceaccount:`+c.n("shop")+`:default" `+
				`cannot list resource "configmaps" in API group "" in `+
				`the namespace "`+c.n("shop")+`"`)
		},
	}
}

// bindingOfAnotherAccountRemoved: api is refused a ConfigMap for its own
// account, which was never granted it. The only RoleBinding deleted in
// the namespace is web's. A forbidden error next to an RBAC deletion is
// not evidence the deletion caused it: the binding names another account.
func bindingOfAnotherAccountRemoved() scenario {
	return scenario{
		expect: expectation{
			Name: "binding-of-another-account-removed",
			Description: "An app is forbidden to read a ConfigMap for " +
				"its own account while only another account's " +
				"RoleBinding was deleted.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"rolebinding/shop/web-config-reader",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			api, _ := accessFleet(c)
			web := accessRoleBinding(c, "web-config-reader", "web")
			c.list(web)
			c.after(time.Minute)
			c.remove(web)
			c.after(2 * time.Minute)
			accessLogCrash(c, api, accessForbiddenLine(c, "api"), 5)
		},
	}
}

// accessForbiddenLine is the log line of a program refused a ConfigMap
// for the named ServiceAccount of shop.
func accessForbiddenLine(c *cluster, account string) string {
	return "level=fatal msg=\"configmaps \\\"api-settings\\\" is " +
		"forbidden: User \\\"system:serviceaccount:" + c.n("shop") + ":" +
		account + "\\\" cannot get resource \\\"configmaps\\\" in " +
		"API group \\\"\\\" in the namespace \\\"" + c.n("shop") +
		"\\\"\""
}

// accessLogCrash crash-loops both replicas of w with an empty
// termination message and records the first error line of the previous
// log, as the crash-log round does.
func accessLogCrash(c *cluster, w *workload, line string, rounds int32) {
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
