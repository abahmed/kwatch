package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// advReachabilityScenarios are changes that sit near a failure in the
// graph or in time but have no evidence of causing it. Each failure's
// own error says something else, or nothing.
func advReachabilityScenarios() []scenario {
	return []scenario{
		advNetPolUnrelatedCrash(), advDependencyRolloutUnrelatedPanic(),
	}
}

// advNetPolUnrelatedCrash: a NetworkPolicy that selects the batch pods
// is created; a minute later the api, which it does not select, panics
// on a nil dereference.
func advNetPolUnrelatedCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-netpol-unrelated-crash",
			Description: "A NetworkPolicy is created for other pods in the " +
				"namespace while an app panics for its own reasons.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"networkpolicy/shop/batch-egress",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			api, web := accessFleet(c)
			c.after(time.Minute)
			policy := clusterEgressPolicy(c, web)
			policy.ObjectMeta = clusterMeta(c, "shop", "batch-egress",
				"netpol")
			policy.Spec.PodSelector = metav1.LabelSelector{
				MatchLabels: map[string]string{"app": c.n("batch")}}
			c.create(policy)
			c.after(time.Minute)
			accessCrash(c, api, "panic: runtime error: invalid memory "+
				"address or nil pointer dereference")
		},
	}
}

// advDependencyRolloutUnrelatedPanic: db, which api calls, completes a
// healthy rollout a minute before api panics on a nil map. db changed
// and is reachable from api in the graph; nothing says it caused the
// panic.
func advDependencyRolloutUnrelatedPanic() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-dependency-rolled-unrelated-panic",
			Description: "A dependency finishes a healthy rollout just " +
				"before an app panics on an unrelated error.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/db", "service/shop/db",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			db := c.deployment("shop", "db", "registry.example.com/db:15.1", 2)
			c.list(db.objects())
			dbPods := []*corev1.Pod{db.pod(0, "n1"), db.pod(1, "n2")}
			c.list(dbPods[0], dbPods[1])
			c.list(clusterService(c, "shop", "db", 5432),
				trafficSlice(c, "db", dbPods...))
			api := c.deployment("shop", "api", "registry.example.com/api:4", 2)
			configTemplate(api, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{{
					Name: "DB_ADDR", Value: "db:5432"}}
			})
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(3 * time.Minute)
			old := []*corev1.Pod{db.pod(0, "n1"), db.pod(1, "n2")}
			rs := db.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/db:15.2"
			})
			c.update(db.deployment)
			c.create(rs)
			fresh := []*corev1.Pod{db.pod(0, "n1", startedNow),
				db.pod(1, "n2", startedNow)}
			c.create(fresh[0], fresh[1])
			c.update(trafficSlice(c, "db", fresh...))
			c.remove(old[0], old[1])
			db.setReady(2)
			c.update(db.objects())
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				message := "panic: assignment to entry in nil map"
				c.update(api.pod(0, "n1", crashLoop(2, "Error", message,
					restarts)), api.pod(1, "n2", crashLoop(2, "Error",
					message, restarts)))
				api.setReady(0)
				c.update(api.objects())
				c.after(time.Minute)
			}
		},
	}
}

var _ = networkingv1.PolicyTypeEgress
