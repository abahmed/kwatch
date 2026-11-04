package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// backendNotReadyScenarios are callers that fail on a Service whose
// only backends are pods of one workload that run but are not ready,
// before anything about those pods has a finding of its own.
func backendNotReadyScenarios() []scenario {
	return []scenario{
		calledBackendNotReady("called-backend-not-ready-timeout",
			"dial tcp ledger:9000: i/o timeout"),
		calledBackendNotReady("called-backend-not-ready-refused",
			"ledger client: dial tcp ledger:9000: connect: "+
				"connection refused"),
	}
}

// calledBackendNotReady: both pods of Deployment ledger keep running
// but stop being ready, so Service ledger has no ready endpoint. The
// orders pods call it by name and crash with callerError. The empty
// Service only says the pods are not ready: the workload is the root,
// and the endpoints alone are not proof enough to say so with high
// confidence.
func calledBackendNotReady(name, callerError string) scenario {
	return scenario{
		expect: expectation{
			Name: name,
			Description: "A Service's backends run but are not ready " +
				"and its callers crash with an error naming it.",
			Root: "deployment/shop/ledger", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"deployment/shop/orders", "node//n1"},
		},
		build: func(c *cluster) {
			buildCalledBackendNotReady(c, callerError)
		},
	}
}

func buildCalledBackendNotReady(c *cluster, callerError string) {
	c.list(c.node("n1", "zone-a"))
	ledger := c.deployment("shop", "ledger",
		"registry.example.com/ledger:3", 2)
	c.list(ledger.objects())
	pods := []*corev1.Pod{ledger.pod(0, "n1"), ledger.pod(1, "n1")}
	c.list(pods[0], pods[1], clusterService(c, "shop", "ledger", 9000),
		trafficSlice(c, "ledger", pods...))
	orders := c.deployment("shop", "orders",
		"registry.example.com/orders:8", 2)
	configTemplate(orders, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "LEDGER_ADDR", Value: c.n("ledger") + ":9000",
		}}
	})
	c.list(orders.objects())
	c.list(orders.pod(0, "n1"), orders.pod(1, "n1"))
	c.after(10 * time.Minute)
	for i := range pods {
		c.update(ledger.pod(i, "n1", notReady))
	}
	ledger.setReady(0)
	c.update(ledger.objects())
	c.update(trafficUnreadySlice(c, "ledger",
		ledger.pod(0, "n1", notReady), ledger.pod(1, "n1", notReady)))
	c.after(30 * time.Second)
	for restarts := int32(1); restarts <= 4; restarts++ {
		for replica := range 2 {
			c.update(orders.pod(replica, "n1", crashLoop(1, "Error",
				callerError, restarts)))
		}
		orders.setReady(0)
		c.update(orders.objects())
		c.after(time.Minute)
	}
}
