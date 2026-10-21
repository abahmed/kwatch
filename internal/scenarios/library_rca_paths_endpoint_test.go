package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rcaEndpointScenarios are pods that time out on an external database
// their configuration names while kwatch's own probe of it connects.
func rcaEndpointScenarios() []scenario {
	return []scenario{
		endpointAnswersProbe(), nodeEgressExternalEndpoint(),
	}
}

const rcaExternalDB = "ext-db.corp.example:5432"

// rcaDatabaseFleet lists three Deployments of "shop" whose containers
// read the external database address from DATABASE_URL, with replicas
// spread over nodes.
func rcaDatabaseFleet(
	c *cluster, replicas int32, nodes []string,
) []*workload {
	var fleet []*workload
	for _, name := range []string{"web", "api", "worker"} {
		w := c.deployment("shop", name,
			"registry.example.com/"+name+":1.0", replicas)
		configTemplate(w, func(spec *corev1.PodSpec) {
			spec.Containers[0].Env = []corev1.EnvVar{{
				Name:  "DATABASE_URL",
				Value: "postgres://app@" + rcaExternalDB + "/app"}}
		})
		c.list(w.objects())
		for i, node := range nodes {
			c.list(w.pod(i, node))
		}
		fleet = append(fleet, w)
	}
	return fleet
}

// endpointAnswersProbe: all replicas of three workloads time out on the
// external database while the probe connects every minute. The database
// answers, so it is not the one that "cannot be reached".
func endpointAnswersProbe() scenario {
	return scenario{
		expect: expectation{
			Name: "endpoint-answers-probe-three-workloads",
			Description: "Three workloads time out on an external " +
				"database that answers kwatch's probe.",
			Root: rootUnknown, Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"external-endpoint//" + rcaExternalDB,
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := rcaDatabaseFleet(c, 2, []string{"n1", "n2"})
			db := inventory.CoreID(kube.KindExternalEndpoint, "",
				rcaExternalDB)
			c.probe(db, "")
			c.after(3 * time.Minute)
			message := "dial tcp " + rcaExternalDB + ": i/o timeout"
			for round := int32(1); round <= 5; round++ {
				c.probe(db, "")
				for _, w := range fleet {
					c.update(
						w.pod(0, "n1", crashLoop(1, "Error", message,
							round)),
						w.pod(1, "n2", crashLoop(1, "Error", message,
							round)))
					w.setReady(0)
					c.update(w.objects())
				}
				c.after(time.Minute)
			}
		},
	}
}

// nodeEgressExternalEndpoint: only the replicas on n3 time out on the
// external database; the probe connects and the replicas on n1 and n2
// work. What the failing pods share is the node.
func nodeEgressExternalEndpoint() scenario {
	return scenario{
		expect: expectation{
			Name: "node-egress-external-endpoint",
			Description: "One node's pods time out on an external " +
				"database that answers kwatch's probe and the other " +
				"nodes' pods.",
			Root: "node//n3", Tier: "notify", MaxMessages: 3,
			Ambiguous: true,
			MustNotBlame: []string{"external-endpoint//" + rcaExternalDB,
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			fleet := rcaDatabaseFleet(c, 3, []string{"n1", "n2", "n3"})
			db := inventory.CoreID(kube.KindExternalEndpoint, "",
				rcaExternalDB)
			c.probe(db, "")
			c.after(3 * time.Minute)
			message := "dial tcp " + rcaExternalDB + ": i/o timeout"
			for round := int32(1); round <= 5; round++ {
				c.probe(db, "")
				for _, w := range fleet {
					c.update(w.pod(2, "n3", crashLoop(1, "Error", message,
						round)))
					w.setReady(2)
					c.update(w.objects())
				}
				c.after(time.Minute)
			}
		},
	}
}
