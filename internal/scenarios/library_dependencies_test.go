package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dependencyScenarios are workloads failing on a dependency outside the
// cluster that their configuration names. kwatch's own probe decides:
// a dependency that refuses connections is the root, even for a single
// workload; one that answers leaves the workload its own problem.
func dependencyScenarios() []scenario {
	return []scenario{dependencyRefusesConnections(), dependencyAnswers()}
}

// ordersDatabase is the database the orders API is configured to call.
const ordersDatabase = "db.example.com:5432"

// dependencyRefusesConnections: the orders API's two replicas crash-loop
// on their database, which the API's DATABASE_URL names and which
// kwatch's probe finds refusing connections. The database is the root.
func dependencyRefusesConnections() scenario {
	return scenario{
		expect: expectation{
			Name: "dependency-refuses-connections",
			Description: "One Deployment crash-loops on the database its " +
				"environment names; kwatch's probe of that endpoint " +
				"is refused.",
			Root: "external-endpoint//" + ordersDatabase, Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"deployment/orders/api", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			api := dependencyApp(c)
			c.after(time.Minute)
			db := inventory.CoreID(kube.KindExternalEndpoint, "",
				ordersDatabase)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.probe(db, "dial tcp: connection refused")
				dependencyCrash(c, api, restarts)
				c.after(time.Minute)
			}
		},
	}
}

// dependencyAnswers: the same crash loop, but the probe reaches the
// database every time. The failure is the API's own.
func dependencyAnswers() scenario {
	return scenario{
		expect: expectation{
			Name: "dependency-answers",
			Description: "One Deployment crash-loops on a database error " +
				"while kwatch's probe of the database succeeds.",
			Root: "deployment/orders/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"external-endpoint//" + ordersDatabase,
				"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			api := dependencyApp(c)
			c.after(time.Minute)
			db := inventory.CoreID(kube.KindExternalEndpoint, "",
				ordersDatabase)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.probe(db, "")
				dependencyCrash(c, api, restarts)
				c.after(time.Minute)
			}
		},
	}
}

// dependencyApp lists the orders API whose containers read the database
// address from DATABASE_URL.
func dependencyApp(c *cluster) *workload {
	api := c.deployment("orders", "api", "registry.example.com/api:3.1", 2)
	api.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name:  "DATABASE_URL",
			Value: "postgres://orders:secret@" + ordersDatabase + "/orders",
		}}
	})
	api.setReady(2)
	c.list(api.objects())
	c.list(api.pod(0, "n1"), api.pod(1, "n1"))
	return api
}

// dependencyCrash crash-loops both replicas on a password error, which
// names nothing on its own.
func dependencyCrash(c *cluster, api *workload, restarts int32) {
	for replica := range 2 {
		c.update(api.pod(replica, "n1", crashLoop(1, "Error",
			"FATAL: could not open database connection", restarts)))
	}
	api.setReady(0)
	c.update(api.objects())
}
