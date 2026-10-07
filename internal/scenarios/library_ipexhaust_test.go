package scenarios

import "time"

const ipExhaustedMessage = "Failed to create pod sandbox: rpc error: code " +
	"= Unknown desc = failed to setup network for sandbox: plugin " +
	"type=\"aws-cni\" failed (add): add cmd: failed to assign an IP " +
	"address to container"

// ipScenarios are pods that cannot get a network address.
func ipScenarios() []scenario {
	return []scenario{podIPsExhausted()}
}

// podIPsExhausted: fourteen pods of three workloads wait in
// ContainerCreating on three nodes whose CNI has no free address left.
func podIPsExhausted() scenario {
	return scenario{
		expect: expectation{
			Name: "pod-ips-exhausted",
			Description: "Fourteen pods of three workloads cannot " +
				"start on three nodes: the CNI has no free pod " +
				"addresses.",
			Root: "zone//zone-a", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"deployment/shop/api", "node//n4"},
			Tail:         duration(10 * time.Minute),
		},
		build: buildIPExhausted,
	}
}

func buildIPExhausted(c *cluster) {
	nodes := []string{"n1", "n2", "n3"}
	for _, name := range nodes {
		c.list(c.node(name, "zone-a"))
	}
	var works []*workload
	for _, name := range []string{"api", "web", "worker"} {
		w := c.deployment("shop", name, "registry.example.com/"+name+":1", 5)
		w.setReady(0)
		c.list(w.objects())
		works = append(works, w)
	}
	c.list(c.node("n4", "zone-b"))
	c.after(time.Minute)
	for i := 0; i < 14; i++ {
		pod := works[(i/3)%3].pod(i, nodes[i%3],
			waiting("ContainerCreating", ""))
		c.list(pod)
		event := c.warningEvent(pod, "Pod", "FailedCreatePodSandBox",
			ipExhaustedMessage, "kubelet", 3)
		event.InvolvedObject.UID = pod.UID
		c.warn(event)
	}
	c.after(5 * time.Minute)
}
