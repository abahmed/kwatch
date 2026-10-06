package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// unschedulableQuantifiedScenarios are pods the scheduler cannot place,
// where the message should say how big the gap is (or that there is no
// resource gap at all).
func unschedulableQuantifiedScenarios() []scenario {
	return []scenario{unschedulableQuantified(), unschedulableTaint()}
}

// requestCPU makes the first container of a pod spec ask for cpu.
func requestCPU(cpu string) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) {
		spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse(cpu),
		}
	}
}

// pendingForMinutes repeats the scheduler warning for one pod.
func pendingForMinutes(
	c *cluster, w *workload, message string, minutes int,
) {
	for m := 1; m <= minutes; m++ {
		c.after(time.Minute)
		pod := last(c, w.pod(0, ""))
		c.warn(c.warningEvent(pod, "Pod", "FailedScheduling", message,
			"default-scheduler", int32(m)))
	}
}

// unschedulableQuantified: three 4-CPU nodes each carry 2.5 CPU of
// requests, so 1.5 CPU is the most any has free; the new pod needs 3.
func unschedulableQuantified() scenario {
	return scenario{
		expect: expectation{
			Name: "unschedulable-quantified",
			Description: "A pod asking for 3 CPU is Pending on a cluster " +
				"whose nodes have at most 1.5 CPU free; the message " +
				"states the need and the most free.",
			Root: "scheduling//Insufficient cpu", Tier: "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			names := []string{"n1", "n2", "n3"}
			for _, name := range names {
				c.list(c.node(name, "zone-a"))
			}
			fill := c.deployment("batch", "filler",
				"registry.example.com/filler:1.0", 3)
			fill.rollout(requestCPU("2500m"))
			fill.setReady(3)
			c.list(fill.objects())
			c.list(fill.pod(0, "n1"), fill.pod(1, "n2"), fill.pod(2, "n3"))
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 1)
			w.rollout(requestCPU("3"))
			w.setReady(0)
			c.create(w.objects())
			message := "0/3 nodes are available: 3 Insufficient cpu. " +
				"preemption: 0/3 nodes are available: 3 No preemption " +
				"victims found for incoming pod."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 6)
		},
	}
}

// unschedulableTaint: the only block is a taint, so no CPU or memory
// numbers may appear.
func unschedulableTaint() scenario {
	return scenario{
		expect: expectation{
			Name: "unschedulable-taint",
			Description: "A pod is Pending because every node carries a " +
				"taint it does not tolerate; there is no resource gap " +
				"to quantify.",
			Root: "scheduling//had untolerated taint", Tier: "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			for _, name := range []string{"t1", "t2"} {
				node := c.node(name, "zone-a")
				node.Spec.Taints = []corev1.Taint{{
					Key: "dedicated", Value: "gpu", Effect: "NoSchedule",
				}}
				c.list(node)
			}
			w := c.deployment("analytics", "report",
				"registry.example.com/report:2.0", 1)
			w.rollout(requestCPU("500m"))
			w.setReady(0)
			c.create(w.objects())
			message := "0/2 nodes are available: 2 node(s) had " +
				"untolerated taint {dedicated: gpu}. preemption: 0/2 " +
				"nodes are available: 2 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 6)
		},
	}
}
