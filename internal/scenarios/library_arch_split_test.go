package scenarios

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"
)

// archSplitScenarios compare healthy and failing replicas of one
// workload: the failing ones all run on arm64 nodes and the healthy
// ones on amd64 nodes, or the failing ones run on both.
func archSplitScenarios() []scenario {
	return []scenario{archSplitReplicas(), archMixedReplicas()}
}

// archSplitReplicas: three of seven api replicas crash with an
// ordinary error, all on arm64 nodes; the four on amd64 nodes are
// ready. The split is an observation; the workload stays the root.
func archSplitReplicas() scenario {
	return scenario{
		expect: expectation{
			Name: "replicas-fail-only-on-arm64",
			Description: "Three replicas on arm64 nodes crash with an " +
				"ordinary error while four on amd64 nodes are ready; " +
				"the comparison names the split, not the node.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//a1", "node//a2"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			buildArchSplit(c, []string{"a1", "a2", "a1"})
		},
	}
}

// archMixedReplicas: the same layout, but one failing replica runs on
// an amd64 node, so no attribute splits failing from healthy.
func archMixedReplicas() scenario {
	return scenario{
		expect: expectation{
			Name: "replicas-fail-on-mixed-nodes",
			Description: "Three replicas crash, two on arm64 nodes and " +
				"one on an amd64 node; no clean split is claimed.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//a1", "node//a2", "node//b1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			buildArchSplit(c, []string{"a1", "a2", "b1"})
		},
	}
}

// buildArchSplit runs api with seven replicas: the failing ones on
// failing, the four healthy ones on amd64 nodes b1 and b2.
func buildArchSplit(c *cluster, failing []string) {
	for _, name := range []string{"a1", "a2", "b1", "b2"} {
		node := c.node(name, "zone-a")
		node.Labels["kubernetes.io/arch"] = "amd64"
		if name[0] == 'a' {
			node.Labels["kubernetes.io/arch"] = "arm64"
		}
		c.list(node)
	}
	api := c.deployment("shop", "api", "ghcr.io/x/api:1.4", 7)
	api.setReady(4)
	c.list(api.objects())
	c.after(time.Minute)
	var pods []runtime.Object
	for i, node := range failing {
		pods = append(pods, api.pod(i, node,
			crashLoop(2, "Error", "panic: nil pointer", 4),
			longRun(2*time.Hour)))
	}
	for i, node := range []string{"b1", "b2", "b1", "b2"} {
		pods = append(pods, api.pod(len(failing)+i, node))
	}
	c.list(pods...)
	c.after(time.Minute)
}
