package scenarios

import (
	corev1 "k8s.io/api/core/v1"
)

// fitVolumeZoneNodeFull: the pod's volume lives in zone-b, whose only
// node has no CPU left. Nothing is broken: the volume is bound and the
// node is up, so the root is the scheduler's capacity, as for any pod
// that no node has room for. The message names the volume and that node.
func fitVolumeZoneNodeFull() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-volume-zone-node-full",
			Description: "A pod is Pending because its bound volume " +
				"can only attach in zone-b, and the only node there " +
				"has no room; the message says so.",
			Root:        "scheduling//Insufficient cpu",
			Tier:        "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(fitPoolNode(c, "a1", "zone-a", "general", "4"),
				fitPoolNode(c, "b1", "zone-b", "general", "4"))
			fitPoolLoad(c, "zone-b-load", "3500m", "b1")
			c.list(scheduleClass(c, "zonal-ssd", "WaitForFirstConsumer"))
			claim := storageBoundClaim(c, "analytics", "data-etl",
				"zonal-ssd", "50Gi", "pv-etl")
			c.list(heldoutZonalVolume(c, claim, "zone-b"), claim)
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 1)
			w.rollout(requestCPU("2"))
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/2 nodes are available: 1 Insufficient cpu, 1 " +
				"node(s) had volume node affinity conflict. preemption: " +
				"0/2 nodes are available: 1 No preemption victims found " +
				"for incoming pod, 1 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 6)
		},
	}
}

// fitLocalVolumeNodeGone: a local volume is tied to node n9, which no
// longer exists.
func fitLocalVolumeNodeGone() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-local-volume-node-gone",
			Description: "A pod is Pending because its local volume is " +
				"tied to a node that is gone; the message says so.",
			Root:        "persistentvolumeclaim/analytics/data-etl",
			Tier:        "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(fitPoolNode(c, "a1", "zone-a", "general", "4"))
			c.list(scheduleClass(c, "local", "WaitForFirstConsumer"))
			claim := storageBoundClaim(c, "analytics", "data-etl",
				"local", "50Gi", "pv-etl")
			volume := heldoutZonalVolume(c, claim, "zone-a")
			term := &volume.Spec.NodeAffinity.Required.
				NodeSelectorTerms[0].MatchExpressions[0]
			term.Key = corev1.LabelHostname
			term.Values = []string{c.n("n9")}
			c.list(volume, claim)
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 1)
			w.rollout(requestCPU("500m"))
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: 1 node(s) had volume " +
				"node affinity conflict. preemption: 0/1 nodes are " +
				"available: 1 Preemption is not helpful for scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 6)
		},
	}
}
