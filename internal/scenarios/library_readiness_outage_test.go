package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// readinessOutageScenarios are dependency outages where most callers
// stay up and fail their readiness probe instead of crashing, on a
// cluster whose pods mostly share one node.
func readinessOutageScenarios() []scenario {
	return []scenario{readinessOutageSharedDatabase(),
		readinessOutageNodeOutOfAddresses(), readinessOutageMemberLeaves()}
}

// outageDatabase is the database outside the cluster the callers name.
const outageDatabase = "10.0.9.40:5432"

// outageLine is what every caller logs while the database refuses it.
const outageLine = "Npgsql.NpgsqlException (0x80004005): Failed to " +
	"connect to " + outageDatabase

// outageCallers are the workloads that call the database: the first
// five stay up and fail readiness, the last two crash on start.
var outageCallers = []string{"accounts", "billing", "fleet", "pricing",
	"tracking", "payments", "reports"}

// readinessOutageSharedDatabase: seven single-replica services that
// all run on n1 call one database outside the cluster. It stops
// accepting connections: five stay up, fail their readiness probe and
// log the failed connection; two crash with the same line. Three
// unrelated pods on n2 stay healthy. One incident blames the database
// endpoint; n1, which merely hosts the callers, is not blamed.
func readinessOutageSharedDatabase() scenario {
	notBlamed := []string{"node//n1", "node//n2"}
	for _, app := range outageCallers {
		notBlamed = append(notBlamed, "deployment/shop/"+app)
	}
	return scenario{
		expect: expectation{
			Name: "readiness-outage-shared-database",
			Description: "Seven services on one node lose their " +
				"database: five stay up and fail readiness, two " +
				"crash, all logging the same failed connection.",
			Root: "external-endpoint//" + outageDatabase, Tier: "notify",
			MaxMessages: 3, MustNotBlame: notBlamed,
			Tail: duration(10 * time.Minute),
		},
		build: buildReadinessOutage,
	}
}

// readinessOutageNodeOutOfAddresses: the same outage, while n1 also
// runs out of pod addresses and three new pods of another workload
// cannot start there. The node explains the pods that cannot get an
// address, never the running services that cannot reach their
// database: those stay with the database endpoint.
func readinessOutageNodeOutOfAddresses() scenario {
	s := readinessOutageSharedDatabase()
	s.expect.Name = "readiness-outage-node-out-of-addresses"
	s.expect.Description = "Seven services on one node lose their " +
		"database while the node also runs out of pod addresses for " +
		"three new pods."
	s.expect.OtherRoots = []string{"node//n1"}
	s.expect.MustNotBlame = s.expect.MustNotBlame[1:]
	s.expect.MaxMessages = 4
	s.build = func(c *cluster) {
		buildReadinessOutage(c)
		w := c.deployment("shop", "batch", "registry.example.com/batch:1",
			3)
		w.setReady(0)
		c.list(w.objects())
		for i := range 3 {
			pod := w.pod(i, "n1", waiting("ContainerCreating", ""))
			c.list(pod)
			ev := c.warningEvent(pod, "Pod", "FailedCreatePodSandBox",
				ipExhaustedMessage, "kubelet", 3)
			ev.InvolvedObject.UID = pod.UID
			c.warn(ev)
		}
		c.after(5 * time.Minute)
	}
	return s
}

// readinessOutageMemberLeaves: the same outage; then the two crashing
// services are redeployed with a setting they reject and crash with an
// error of their own. They leave the database's incident, whose title
// and steps must no longer name them.
func readinessOutageMemberLeaves() scenario {
	s := readinessOutageSharedDatabase()
	s.expect.Name = "readiness-outage-member-leaves"
	s.expect.Description = "Seven services lose their database; two " +
		"of them then crash on a setting of their own and leave the " +
		"database's incident."
	s.expect.OtherRoots = []string{"deployment/shop/payments",
		"deployment/shop/reports"}
	s.expect.MustNotBlame = []string{"node//n1", "node//n2"}
	s.expect.MaxMessages = 6
	s.build = func(c *cluster) {
		fleet := buildReadinessOutageFleet(c)
		for restarts := int32(4); restarts <= 9; restarts++ {
			for _, w := range fleet[5:] {
				c.update(w.pod(0, "n1", crashLoop(1, "Error",
					"fatal: unknown setting \"cache.mode\" in "+
						"/etc/"+w.deployment.Name+".yaml", restarts)))
			}
			c.after(time.Minute)
		}
		c.after(5 * time.Minute)
	}
	return s
}

func buildReadinessOutage(c *cluster) {
	buildReadinessOutageFleet(c)
}

// buildReadinessOutageFleet runs the outage and returns the callers.
func buildReadinessOutageFleet(c *cluster) []*workload {
	return buildOutageWith(c, nil)
}

// buildOutageWith runs the outage; failing, when set, runs once the
// callers have begun to fail, as a prober that reacts to them would.
func buildOutageWith(c *cluster, failing func(*cluster)) []*workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	var fleet []*workload
	for _, app := range outageCallers {
		w := c.deployment("shop", app, "registry.example.com/"+app+":4", 1)
		c.list(w.objects())
		c.list(w.pod(0, "n1"))
		fleet = append(fleet, w)
	}
	for _, app := range []string{"search", "web", "docs"} {
		w := c.deployment("shop", app, "registry.example.com/"+app+":2", 1)
		c.list(w.objects())
		c.list(w.pod(0, "n2"))
	}
	c.after(5 * time.Minute)
	began := c.now
	for count := int32(3); count <= 9; count += 3 {
		for i, w := range fleet {
			if i >= 5 {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", outageLine,
					count/3)))
			} else {
				readinessFailing(c, w, began, count)
			}
			w.setReady(0)
			c.update(w.deployment, w.replicaSet)
		}
		if failing != nil && count == 3 {
			failing(c)
		}
		c.after(time.Minute)
	}
	c.after(5 * time.Minute)
	return fleet
}

// readinessFailing marks w's pod on n1 running but not ready since
// began, with count failed readiness probes and the outage line in its
// current log, as the crash-log round records it.
func readinessFailing(c *cluster, w *workload, began time.Time,
	count int32) {
	pod := w.pod(0, "n1", notReadySince(began))
	c.update(pod)
	ev := c.warningEvent(pod, "Pod", "Unhealthy", "Readiness probe "+
		"failed: HTTP probe failed with statuscode: 503", "kubelet", count)
	ev.InvolvedObject.FieldPath = "spec.containers{app}"
	c.warn(ev)
	c.emit(kube.ProbeLogObservation(containerOf(pod), c.now,
		[]string{"info: GET /health/ready 503", outageLine}))
}

// containerOf is the ID of a pod's first container.
func containerOf(pod *corev1.Pod) inventory.EntityID {
	return kube.ContainerID(pod.Namespace, pod.Name,
		pod.Spec.Containers[0].Name)
}
