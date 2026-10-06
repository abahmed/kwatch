package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// schedulingFitScenarios are pods that fit no node, where the message
// says which pools were checked and what stops each, and what the
// autoscaler said about adding a node.
func schedulingFitScenarios() []scenario {
	return []scenario{
		fitCPUNearMiss(), fitTaintMismatch(), fitVolumeZoneNoNodes(),
		fitAutoscalerScalingUp(), fitAutoscalerCannotScale(),
	}
}

// fitPoolNode is a node of a Karpenter pool with cpu CPUs.
func fitPoolNode(
	c *cluster, name, zone, pool, cpu string, taints ...corev1.Taint,
) *corev1.Node {
	node := c.node(name, zone)
	node.Labels["karpenter.sh/nodepool"] = pool
	node.Status.Allocatable[corev1.ResourceCPU] = resource.MustParse(cpu)
	node.Spec.Taints = taints
	return node
}

// fitPoolLoad lists a filler workload whose pods ask cpu each, one per
// node, so the node has what is left of its CPU free.
func fitPoolLoad(c *cluster, name, cpu string, nodes ...string) {
	fill := c.deployment("batch", name, "registry.example.com/"+name+":1",
		int32(len(nodes)))
	fill.rollout(requestCPU(cpu))
	fill.setReady(int32(len(nodes)))
	c.list(fill.objects())
	for i, node := range nodes {
		c.list(fill.pod(i, node))
	}
}

// normalEvent is a Normal event, as an autoscaler writes its decisions.
func (c *cluster) normalEvent(
	obj metav1.Object, reason, message, source string, count int32,
) *corev1.Event {
	ev := c.warningEvent(obj, "Pod", reason, message, source, count)
	ev.Type = corev1.EventTypeNormal
	return ev
}

// fitCPUNearMiss: two pools, each just short of the 1.5 CPU the new pod
// asks; the message names the best node of each.
func fitCPUNearMiss() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-cpu-near-miss",
			Description: "A pod asking for 1.5 CPU is Pending; pool " +
				"general's best node has 1.2 CPU free and pool bulk's " +
				"has 1; the message names both near misses.",
			Root: "scheduling//Insufficient cpu", Tier: "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(fitPoolNode(c, "n1", "zone-a", "general", "4"),
				fitPoolNode(c, "n3", "zone-a", "general", "4"),
				fitPoolNode(c, "big1", "zone-b", "bulk", "8"))
			fitPoolLoad(c, "general-load", "2800m", "n1", "n3")
			fitPoolLoad(c, "bulk-load", "7", "big1")
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 1)
			w.rollout(requestCPU("1500m"))
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

// fitTaintMismatch: the general pool is full and the gpu pool has room
// behind a taint the pod does not tolerate.
func fitTaintMismatch() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-taint-mismatch",
			Description: "A pod is Pending because the general pool is " +
				"full and the gpu pool, which has room, is tainted; the " +
				"message says the taint is the blocker and that " +
				"tolerating it would fit.",
			Root: "scheduling//Insufficient cpu", Tier: "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			gpuTaint := corev1.Taint{Key: "nvidia.com/gpu", Value: "true",
				Effect: corev1.TaintEffectNoSchedule}
			c.list(fitPoolNode(c, "g1", "zone-a", "general", "4"),
				fitPoolNode(c, "gpu-1", "zone-a", "gpu", "8", gpuTaint))
			fitPoolLoad(c, "general-load", "3500m", "g1")
			w := c.deployment("analytics", "report",
				"registry.example.com/report:2.0", 1)
			w.rollout(requestCPU("2"))
			w.setReady(0)
			c.create(w.objects())
			message := "0/2 nodes are available: 1 Insufficient cpu, 1 " +
				"node(s) had untolerated taint {nvidia.com/gpu: true}. " +
				"preemption: 0/2 nodes are available: 1 No preemption " +
				"victims found for incoming pod, 1 Preemption is not " +
				"helpful for scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 6)
		},
	}
}

// fitVolumeZoneNoNodes: the pod's volume lives in a zone with no nodes.
func fitVolumeZoneNoNodes() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-volume-zone-no-nodes",
			Description: "A pod is Pending because its bound volume can " +
				"only attach in zone-b, where the cluster has no nodes; " +
				"the message says so.",
			Root:        "persistentvolumeclaim/analytics/data-etl",
			Tier:        "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(fitPoolNode(c, "a1", "zone-a", "general", "4"),
				fitPoolNode(c, "c1", "zone-c", "general", "4"))
			c.list(scheduleClass(c, "zonal-ssd", "WaitForFirstConsumer"))
			claim := storageBoundClaim(c, "analytics", "data-etl",
				"zonal-ssd", "50Gi", "pv-etl")
			c.list(heldoutZonalVolume(c, claim, "zone-b"), claim)
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 1)
			w.rollout(requestCPU("500m"))
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/2 nodes are available: 2 node(s) had volume " +
				"node affinity conflict. preemption: 0/2 nodes are " +
				"available: 2 Preemption is not helpful for scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 6)
		},
	}
}

// fitAutoscalerScalingUp: the pod is unschedulable for several minutes,
// the autoscaler says it is adding a node, and the pod starts on it.
func fitAutoscalerScalingUp() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-autoscaler-scaling-up",
			Description: "A pod is Pending for six minutes while the " +
				"cluster autoscaler reports TriggeredScaleUp; a node " +
				"joins and the pod starts. Nothing is sent.",
			Quiet: true,
		},
		build: func(c *cluster) {
			c.list(fitPoolNode(c, "n1", "zone-a", "general", "4"))
			fitPoolLoad(c, "general-load", "3500m", "n1")
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 1)
			w.rollout(requestCPU("2"))
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: 1 Insufficient cpu."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 1)
			c.warn(c.normalEvent(last(c, w.pod(0, "")), "TriggeredScaleUp",
				"pod triggered scale-up: [{general-asg 1->2 (max: 5)}]",
				"cluster-autoscaler", 1))
			pendingForMinutes(c, w, message, 5)
			c.list(fitPoolNode(c, "n2", "zone-a", "general", "4"))
			w.setReady(1)
			c.update(w.objects())
			c.update(w.pod(0, "n2", startedNow))
			c.after(10 * time.Minute)
		},
	}
}

// fitAutoscalerCannotScale: the autoscaler says it cannot add a node.
func fitAutoscalerCannotScale() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-autoscaler-cannot-scale",
			Description: "A pod is Pending and the cluster autoscaler " +
				"reports NotTriggerScaleUp because the node group is at " +
				"its maximum; the message quotes it.",
			Root: "scheduling//Insufficient cpu", Tier: "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(fitPoolNode(c, "n1", "zone-a", "general", "4"))
			fitPoolLoad(c, "general-load", "3500m", "n1")
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 1)
			w.rollout(requestCPU("2"))
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: 1 Insufficient cpu."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				pod := last(c, w.pod(0, ""))
				c.warn(c.warningEvent(pod, "Pod", "FailedScheduling",
					message, "default-scheduler", m))
				c.warn(c.normalEvent(pod, "NotTriggerScaleUp",
					"pod didn't trigger scale-up: 1 max node group size "+
						"reached", "cluster-autoscaler", m))
			}
		},
	}
}
