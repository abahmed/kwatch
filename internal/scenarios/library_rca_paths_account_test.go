package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// rcaAccountScenarios are pods refused because the account they run as
// is gone.
func rcaAccountScenarios() []scenario {
	return []scenario{serviceAccountDeleted()}
}

// serviceAccountDeleted: GitOps prunes the ServiceAccount api; the
// running pods keep working, but the scale-out that follows is refused
// by the API server for want of the account. The ReplicaSet that cannot
// create pods and its Deployment are one incident.
func serviceAccountDeleted() scenario {
	return scenario{
		expect: expectation{
			Name: "service-account-deleted-before-scale-out",
			Description: "A ServiceAccount is deleted; the next " +
				"scale-out cannot create pods because it is not found.",
			Root: "serviceaccount/shop/api", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api", "node//n1",
				"node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			sa := &corev1.ServiceAccount{ObjectMeta: c.meta("shop", "api")}
			api := c.deployment("shop", "api",
				"registry.example.com/api:4.2", 2)
			configTemplate(api, func(spec *corev1.PodSpec) {
				spec.ServiceAccountName = sa.Name
			})
			c.list(sa)
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(2 * time.Minute)
			c.remove(sa)
			c.after(3 * time.Minute)
			setReplicas(api, 4)
			api.setReady(2)
			c.update(api.objects())
			message := "Error creating: pods \"" + api.replicaSet.Name +
				"-\" is forbidden: error looking up service account " +
				c.n("shop") + "/api: serviceaccount \"api\" not found"
			for n := int32(1); n <= 5; n++ {
				c.warn(c.warningEvent(api.replicaSet, "ReplicaSet",
					"FailedCreate", message, "replicaset-controller",
					n*2))
				c.after(time.Minute)
			}
		},
	}
}
