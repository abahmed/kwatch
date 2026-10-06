package scenarios

import (
	"fmt"
	"time"
)

// namespaceOutageScenarios are many workloads of one namespace failing
// with nothing in common: the engine finds no shared cause, and people
// hear about the namespace once instead of once per workload.
func namespaceOutageScenarios() []scenario {
	return []scenario{namespaceOutageGroups(), twoFailuresStaySeparate()}
}

// outageFleet lists n single-replica Deployments in shop, the app i on
// its own node ni, so the failures share no node.
func outageFleet(c *cluster, n int) ([]*workload, []string) {
	var fleet []*workload
	var nodes []string
	for i := range n {
		node := fmt.Sprintf("n%d", i)
		c.list(c.node(node, "zone-a"))
		w := c.deployment("shop", fmt.Sprintf("app%d", i),
			fmt.Sprintf("registry.example.com/app%d:1.0", i), 1)
		c.list(w.deployment, w.replicaSet, w.pod(0, node))
		fleet, nodes = append(fleet, w), append(nodes, node)
	}
	return fleet, nodes
}

// outageDependencies name what each app of the fleet lost. The errors
// must differ by more than a number: names with digits read as one error.
var outageDependencies = []string{"cache", "queue", "mailer", "vault",
	"ledger", "geocoder", "scheduler", "indexer", "gateway", "notifier",
	"renderer", "importer", "exporter", "resolver", "tracker"}

// outageCrash crash-loops app i of the fleet with its own error.
func outageCrash(
	c *cluster, w *workload, node string, i int, restarts int32,
) {
	c.update(w.pod(0, node, crashLoop(1, "Error",
		"fatal: "+outageDependencies[i]+" lost its own dependency",
		restarts)))
	w.setReady(0)
	c.update(w.objects())
}

// namespaceOutageGroups: eight of fifteen Deployments in shop crash-loop
// within seconds of each other, each on its own node with its own error.
// No cause ties them together, so one message names the namespace.
func namespaceOutageGroups() scenario {
	return scenario{
		expect: expectation{
			Name: "namespace-outage-groups",
			Description: "Eight of fifteen Deployments in one namespace " +
				"crash-loop within seconds of each other, on separate " +
				"nodes and with separate errors: one namespace message.",
			Root: "deployment/shop/app0",
			OtherRoots: []string{"deployment/shop/app1",
				"deployment/shop/app2", "deployment/shop/app3",
				"deployment/shop/app4", "deployment/shop/app5",
				"deployment/shop/app6", "deployment/shop/app7"},
			Tier: "notify", MaxMessages: 1,
			MustNotBlame: []string{"node//n0", "node//n1", "zone//zone-a"},
		},
		build: func(c *cluster) {
			fleet, nodes := outageFleet(c, 15)
			c.after(2 * time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for i := range 8 {
					outageCrash(c, fleet[i], nodes[i], i, restarts)
					c.after(time.Second)
				}
				c.after(20 * time.Second)
			}
		},
	}
}

// twoFailuresStaySeparate: two of fifteen Deployments crash-loop minutes
// apart. Two is no outage, so each gets its own message.
func twoFailuresStaySeparate() scenario {
	return scenario{
		expect: expectation{
			Name: "namespace-two-failures-stay-separate",
			Description: "Two of fifteen Deployments in one namespace " +
				"crash-loop minutes apart: each is its own message.",
			Root:       "deployment/shop/app0",
			OtherRoots: []string{"deployment/shop/app1"},
			Tier:       "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//n0", "node//n1", "zone//zone-a"},
		},
		build: func(c *cluster) {
			fleet, nodes := outageFleet(c, 15)
			c.after(2 * time.Minute)
			for i := range 2 {
				for restarts := int32(1); restarts <= 5; restarts++ {
					outageCrash(c, fleet[i], nodes[i], i, restarts)
					c.after(20 * time.Second)
				}
				c.after(3 * time.Minute)
			}
		},
	}
}
