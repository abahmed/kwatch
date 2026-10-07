package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

const execFormatMessage = "exec /app/api: exec format error"

// archScenarios are containers that cannot run on the CPU architecture
// of their node.
func archScenarios() []scenario {
	return []scenario{execFormatOnArm(), execFormatOnOtherError()}
}

// execFormatOnArm: one api replica was scheduled on an arm64 node and
// crashes with "exec format error"; the two on amd64 nodes run fine.
func execFormatOnArm() scenario {
	return scenario{
		expect: expectation{
			Name: "exec-format-error-arm64",
			Description: "A replica on an arm64 node crashes with " +
				"\"exec format error\" while its siblings on amd64 " +
				"nodes run fine.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildArch(c, execFormatMessage) },
	}
}

// execFormatOnOtherError: the same layout, but the crash is an
// ordinary error; the node architecture is not blamed.
func execFormatOnOtherError() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-on-arm64-node-other-error",
			Description: "A replica on an arm64 node crashes with an " +
				"unrelated error; the architecture is not named.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildArch(c, "panic: nil pointer") },
	}
}

func buildArch(c *cluster, message string) {
	arm := c.node("n1", "zone-a")
	arm.Labels["kubernetes.io/arch"] = "arm64"
	nodes := []*corev1.Node{arm}
	for _, name := range []string{"n2", "n3"} {
		node := c.node(name, "zone-a")
		node.Labels["kubernetes.io/arch"] = "amd64"
		nodes = append(nodes, node)
	}
	for _, node := range nodes {
		c.list(node)
	}
	api := c.deployment("shop", "api", "ghcr.io/x/api:1.4", 3)
	api.setReady(2)
	c.list(api.objects())
	c.after(time.Minute)
	c.list(api.pod(0, "n1", crashLoop(255, "Error", message, 4),
		longRun(2*time.Hour)),
		api.pod(1, "n2"), api.pod(2, "n3"))
	c.after(time.Minute)
}
