package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// resolveCauseScenarios are incidents that end while their stated cause
// is looked at again: it is still there, or it recovered.
func resolveCauseScenarios() []scenario {
	return []scenario{
		dependencyRecoversWithCause(), dependencyHealthyCauseRemains(),
	}
}

// dependencyHealthyCauseRemains: the orders API crash-loops on a database
// that refuses connections; a restart gets the replicas running again
// while kwatch's probe of the database is still refused. The database's
// own finding holds the incident open, so nothing is resolved while the
// cause is there: the engine says "healthy again" only once it is not.
func dependencyHealthyCauseRemains() scenario {
	return scenario{
		expect: expectation{
			Name: "resolve-cause-still-present",
			Description: "The replicas recover after a restart while the " +
				"database they crashed on still refuses connections.",
			Root: "external-endpoint//" + ordersDatabase, Tier: "notify",
			MaxMessages:  1,
			MustNotBlame: []string{"deployment/orders/api", "node//n1"},
		},
		build: func(c *cluster) {
			api, db := crashedOrders(c)
			for range 8 {
				c.probe(db, "dial tcp: connection refused")
				healthyOrders(c, api)
				c.after(time.Minute)
			}
		},
	}
}

// dependencyRecoversWithCause: the same story, but the database answers
// again before the replicas recover.
func dependencyRecoversWithCause() scenario {
	return scenario{
		expect: expectation{
			Name: "resolve-cause-recovered",
			Description: "The database answers again and the replicas " +
				"recover; the resolve says it accepts connections.",
			Root: "external-endpoint//" + ordersDatabase, Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"deployment/orders/api", "node//n1"},
		},
		build: func(c *cluster) {
			api, db := crashedOrders(c)
			for range 8 {
				c.probe(db, "")
				healthyOrders(c, api)
				c.after(time.Minute)
			}
		},
	}
}

// crashedOrders crash-loops the orders API for five minutes on a refused
// database and returns the API and the database.
func crashedOrders(c *cluster) (*workload, inventory.EntityID) {
	c.list(c.node("n1", "zone-a"))
	api := dependencyApp(c)
	c.after(time.Minute)
	db := inventory.CoreID(kube.KindExternalEndpoint, "", ordersDatabase)
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.probe(db, "dial tcp: connection refused")
		dependencyCrash(c, api, restarts)
		c.after(time.Minute)
	}
	return api, db
}

// healthyOrders makes both replicas run and become ready.
func healthyOrders(c *cluster, api *workload) {
	for replica := range 2 {
		c.update(api.pod(replica, "n1"))
	}
	api.setReady(2)
	c.update(api.objects())
}
