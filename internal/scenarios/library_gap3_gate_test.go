package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gap3Gated is a pod held by a scheduling gate.
func gap3Gated(c *cluster, pod *corev1.Pod) {
	gap2NoScheduler(c, pod)
	pod.Spec.SchedulingGates = []corev1.PodSchedulingGate{{
		Name: "example.com/queue-admission"}}
	pod.Status.Conditions = []corev1.PodCondition{{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
		Reason:  corev1.PodReasonSchedulingGated,
		Message: "Scheduling is blocked due to non-empty scheduling gates",
	}}
}

// gap3SchedulingGated: a queueing controller put a scheduling gate on the
// pods of worker and was uninstalled; the pods stay Pending and the
// scheduler never says anything.
func gap3SchedulingGated() scenario {
	return scenario{
		expect: g2Exp("pod-scheduling-gate-never-removed",
			"Pods carry a scheduling gate whose controller is gone; "+
				"they stay Pending with SchedulingGated and no "+
				"scheduling event.", "deployment/shop/worker", "notify",
			"node//n1").tail(30 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "worker",
				"registry.example.com/worker:4", 2)
			w.setReady(0)
			c.create(w.objects())
			pods := []*corev1.Pod{w.pod(0, "", gap3Gated),
				w.pod(1, "", gap3Gated)}
			c.create(pods[0], pods[1])
			for range 5 {
				c.after(6 * time.Minute)
				c.update(pods[0].DeepCopy(), pods[1].DeepCopy())
			}
		},
	}
}

// gap3SchedulingGateRemoved: the same gate is removed after 30 seconds
// and the pods run.
func gap3SchedulingGateRemoved() scenario {
	return scenario{
		expect: g2Quiet("pod-scheduling-gate-removed-quickly-quiet",
			"Pods are gated for 30 seconds by a queueing controller and "+
				"run once it lifts the gate."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "worker",
				"registry.example.com/worker:4", 2)
			w.setReady(0)
			c.create(w.objects())
			c.create(w.pod(0, "", gap3Gated), w.pod(1, "", gap3Gated))
			c.after(30 * time.Second)
			c.update(w.pod(0, "n1", startedNow), w.pod(1, "n1", startedNow))
			w.setReady(2)
			c.update(w.objects())
			c.after(10 * time.Minute)
		},
	}
}
