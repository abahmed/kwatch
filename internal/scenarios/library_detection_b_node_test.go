package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

func detectionBNodeScenarios() []scenario {
	return []scenario{nodeReadonlyFilesystem()}
}

// nodeReadonlyFilesystem: a node problem reporter sets ReadonlyFilesystem
// on a node whose pods all run. Nothing fails yet, so it is one digest
// line that quotes the reporter's message, not a silent condition.
func nodeReadonlyFilesystem() scenario {
	return scenario{
		expect: expectation{
			Name: "node-readonly-filesystem",
			Description: "A node problem reporter sets " +
				"ReadonlyFilesystem on a node whose pods run: one " +
				"digest line with the reporter's message.",
			Root: "node//n1", Tier: "digest", MaxMessages: 2,
			Tail:         duration(time.Hour),
			MustNotBlame: []string{"deployment/shop/api"},
		},
		build: buildNodeReadonlyFilesystem,
	}
}

func buildNodeReadonlyFilesystem(c *cluster) {
	n1 := c.node("n1", "zone-a")
	c.list(n1, c.node("n2", "zone-a"))
	api := c.deployment("shop", "api", "registry.example.com/api:3", 2)
	c.list(api.objects())
	c.list(api.pod(0, "n1"), api.pod(1, "n2"))
	c.after(30 * time.Minute)
	setNodeCondition(n1, "ReadonlyFilesystem", corev1.ConditionTrue,
		"FilesystemIsReadOnly",
		"Remounted filesystem read-only: /dev/nvme0n1p1", c.now)
	c.update(n1)
	c.after(20 * time.Minute)
}
