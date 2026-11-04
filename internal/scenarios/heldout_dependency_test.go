package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// heldoutDependencyScenarios are held-out crashes whose previous log
// names a dependency the program could not reach, in stacks other than
// the labelled ones. They were written on 2026-11-03 and their labels
// were fixed before they were first replayed. See heldoutLibrary for
// the rule they follow.
func heldoutDependencyScenarios() []scenario {
	return []scenario{heldoutJDBCRefusedHealthyDB(),
		heldoutQueueTimeoutBackendDown()}
}

// heldoutJDBCRefusedHealthyDB: a Java service's two replicas exit 1 on
// start; only the previous log says why, a PSQLException refusing the
// connection to db:5432. The db Service has a ready endpoint and its pod
// is healthy, so nothing in the cluster is down: a person says the
// service cannot reach its database (its own address or port), and
// blames the service, not the database.
func heldoutJDBCRefusedHealthyDB() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-jdbc-refused-healthy-db",
			Description: "A Java service exits 1 on start with a " +
				"PSQLException refusing its connection to db:5432; the " +
				"db Service and its pod are healthy.",
			Root: "deployment/sales/orders", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"service/sales/db",
				"deployment/sales/db", "node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			db := c.deployment("sales", "db",
				"registry.example.com/postgres:16", 1)
			c.list(db.objects())
			dbPod := db.pod(0, "n2")
			c.list(dbPod, clusterService(c, "sales", "db", 5432),
				portSlice(c, "sales", "db", 5432, true, dbPod))
			orders := c.deployment("sales", "orders",
				"registry.example.com/orders:3.2", 2)
			c.list(orders.objects())
			c.list(orders.pod(0, "n1"), orders.pod(1, "n2"))
			c.after(4 * time.Minute)
			line := "org.postgresql.util.PSQLException: Connection to " +
				"db:5432 refused. Check that the hostname and port are " +
				"correct and that the postmaster is accepting TCP/IP " +
				"connections."
			crashWithLog(c, orders, []string{"n1", "n2"}, 1,
				[]string{"Starting OrdersApplication", line,
					"\tat org.postgresql.core.v3.ConnectionFactoryImpl." +
						"openConnectionImpl(ConnectionFactoryImpl.java:" +
						"346)"})
		},
	}
}

// heldoutQueueTimeoutBackendDown: a Go worker exits 2 with "dial tcp
// queue:5672: i/o timeout" in its previous log. The queue Service has no
// ready endpoint: its only pod runs but fails readiness. A person blames
// the queue Deployment whose pod is not ready, not the worker.
func heldoutQueueTimeoutBackendDown() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-queue-timeout-backend-down",
			Description: "A Go worker exits 2 on a timed-out dial to " +
				"queue:5672 while the queue Service's only pod is not " +
				"ready.",
			Root: "deployment/pipeline/queue", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/pipeline/ingest",
				"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			queue := c.deployment("pipeline", "queue",
				"registry.example.com/rabbitmq:3.13", 1)
			c.list(queue.objects())
			qPod := queue.pod(0, "n1")
			c.list(qPod, clusterService(c, "pipeline", "queue", 5672),
				portSlice(c, "pipeline", "queue", 5672, true, qPod))
			ingest := c.deployment("pipeline", "ingest",
				"registry.example.com/ingest:0.9", 1)
			c.list(ingest.objects())
			c.list(ingest.pod(0, "n1"))
			c.after(3 * time.Minute)
			down := queue.pod(0, "n1", notReady)
			c.update(down, portSlice(c, "pipeline", "queue", 5672, false,
				down))
			queue.setReady(0)
			c.update(queue.objects())
			c.after(time.Minute)
			crashWithLog(c, ingest, []string{"n1"}, 2,
				[]string{"level=info msg=\"connecting\"",
					"level=fatal msg=\"broker: dial tcp queue:5672: i/o " +
						"timeout\""})
		},
	}
}

// portSlice is a Service's EndpointSlice on port with the pods as its
// endpoints, ready or not.
func portSlice(
	c *cluster, namespace, service string, port int32, ready bool,
	pods ...*corev1.Pod,
) *discoveryv1.EndpointSlice {
	slice := clusterSlice(c, namespace, service, pods...)
	slice.Ports = []discoveryv1.EndpointPort{{Port: &port}}
	for i := range slice.Endpoints {
		slice.Endpoints[i].Conditions.Ready = boolPtr(ready)
	}
	return slice
}

// crashWithLog restarts the workload's replicas on nodes five times with
// exit code and no termination message, recording the previous log the
// crash-log round would read after each restart.
func crashWithLog(
	c *cluster, w *workload, nodes []string, code int32, log []string,
) {
	for restarts := int32(1); restarts <= 5; restarts++ {
		for replica, node := range nodes {
			pod := w.pod(replica, node, crashLoop(code, "Error", "",
				restarts))
			c.update(pod)
			c.emit(kube.CrashLogObservation(kube.ContainerID(
				pod.Namespace, pod.Name, "app"), c.now, log))
		}
		w.setReady(0)
		c.update(w.objects())
		c.after(time.Minute)
	}
}
