package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
)

func detectionBWorkloadScenarios() []scenario {
	return []scenario{daemonSetMisscheduledStays()}
}

// daemonSetMisscheduledStays: a DaemonSet reports two nodes that run its
// pod although it is not meant to run there, and the count stays. Every
// replica it wants is ready, so nothing fails: one digest line.
func daemonSetMisscheduledStays() scenario {
	return scenario{
		expect: expectation{
			Name: "daemonset-misscheduled-stays",
			Description: "A DaemonSet whose status keeps reporting " +
				"misscheduled nodes while every wanted pod is ready: " +
				"one digest line.",
			Root: "daemonset/monitoring/agent", Tier: "digest",
			MaxMessages: 2, Tail: duration(time.Hour),
		},
		build: buildDaemonSetMisscheduled,
	}
}

func buildDaemonSetMisscheduled(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	labels := map[string]string{"app": "agent"}
	ds := &appsv1.DaemonSet{
		ObjectMeta: c.meta("monitoring", "agent"),
		Spec: appsv1.DaemonSetSpec{
			Template: podTemplate(labels, "registry.example.com/agent:1"),
		},
	}
	ds.Generation, ds.Status.ObservedGeneration = 1, 1
	setAgentCounts(ds, 2, 2)
	ds.Status.NumberMisscheduled = 2
	c.list(ds, agentPod(c, ds, "n1"), agentPod(c, ds, "n2"))
	c.after(30 * time.Minute)
}
