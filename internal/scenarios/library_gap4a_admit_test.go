package scenarios

import (
	"time"
)

// gap4aAdmitScenarios are pods a kubelet rejects at admission, one after
// another, and the pods it evicts to admit a critical one.
func gap4aAdmitScenarios() []scenario {
	return []scenario{
		gap4aCriticalPreempts(), gap4aTopologyAffinity(), gap4aSMTAlignment(),
		gap4aOSMismatch(), gap4aFeatureUnsupported(), gap4aShutdownReject(),
	}
}

// gap4aScaleUp lists a two-replica Deployment of the shop on n1 and n2 and
// scales it to three at the start of the story.
func gap4aScaleUp(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(10 * time.Minute)
	*w.deployment.Spec.Replicas = 3
	w.setReady(2)
	c.update(w.objects())
	return w
}

// gap4aRejected builds a scenario in which the kubelet of n2 rejects every
// pod the ReplicaSet creates, six times a minute apart; the ReplicaSet
// replaces each at once.
func gap4aRejected(e expectation, reason, message string) scenario {
	return scenario{expect: e.tail(10 * time.Minute), build: func(c *cluster) {
		w := gap4aScaleUp(c)
		for i := 2; i < 8; i++ {
			pod := w.pod(i, "n2", g2Failed(reason, message))
			c.create(pod)
			gapKubelet(c, pod, reason, message, 1)
			c.after(time.Minute)
		}
	}}
}

// gap4aCriticalPreempts: an upgraded node agent with system-node-critical
// priority arrives on a full node; the kubelet kills two application pods
// to admit it and their replacements find no room elsewhere.
func gap4aCriticalPreempts() scenario {
	e := g2Exp("kubelet-preempts-pods-for-critical-agent",
		"A node agent of critical priority is admitted on a full node by "+
			"killing two application pods; their replacements stay "+
			"Pending for lack of CPU.",
		"daemonset/kube-system/node-agent", "notify", "deployment/shop/orders",
		"deployment/billing/payments").budget(3).tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{expect: e, build: func(c *cluster) {
		c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
		orders := c.deployment("shop", "orders",
			"registry.example.com/orders:2", 1)
		payments := c.deployment("billing", "payments",
			"registry.example.com/payments:2", 1)
		c.list(orders.objects())
		c.list(payments.objects())
		c.list(orders.pod(0, "n1"), payments.pod(0, "n1"))
		ds := g2Daemon(c, "kube-system", "node-agent", "n2")
		c.after(10 * time.Minute)
		message := "Preempted in order to admit critical pod"
		setAgentCounts(ds, 2, 2)
		c.update(ds)
		c.create(agentPod(c, ds, "n1"))
		for _, w := range []*workload{orders, payments} {
			pod := w.pod(0, "n1", g2Failed("Preempting", message))
			c.update(pod)
			gapKubelet(c, pod, "Preempting", message, 1)
			w.setReady(0)
			c.update(w.objects())
		}
		c.after(30 * time.Second)
		stuck := "0/2 nodes are available: 2 Insufficient cpu. preemption: " +
			"0/2 nodes are available: 2 No preemption victims found for " +
			"incoming pod."
		for _, w := range []*workload{orders, payments} {
			c.create(w.pod(1, "", pendingUnscheduled(stuck)))
		}
		c.after(5 * time.Minute)
	}}
}

// gap4aTopologyAffinity: the kubelet of n2 runs the single-numa-node
// topology policy and rejects every pod that needs CPUs and a device from
// different NUMA nodes; the scheduler keeps binding pods there.
func gap4aTopologyAffinity() scenario {
	return gap4aRejected(g2Exp("kubelet-topology-affinity-error-pileup",
		"A node's topology manager rejects the pods bound to it with "+
			"TopologyAffinityError; the ReplicaSet replaces each and "+
			"failed pods pile up.",
		"node//n2", "notify", "deployment/shop/api").budget(3),
		"TopologyAffinityError", "Resources cannot be allocated with "+
			"Topology locality")
}

// gap4aSMTAlignment: a Guaranteed pod asks for three CPUs on a node whose
// CPU manager requires whole physical cores.
func gap4aSMTAlignment() scenario {
	e := g2Exp("kubelet-smt-alignment-error",
		"A Guaranteed pod requests three CPUs on a node with the "+
			"full-pcpus-only policy; the kubelet rejects each pod with "+
			"SMTAlignmentError.",
		"deployment/shop/api", "notify").budget(3)
	e.Ambiguous = true
	return gap4aRejected(e, "SMTAlignmentError", "SMT Alignment Error: "+
		"requested 3 cpus not multiple of cpus per core = 2")
}

// gap4aOSMismatch: pods whose spec names the Windows OS are bound to a
// Linux node; the kubelet rejects them.
func gap4aOSMismatch() scenario {
	return gap4aRejected(g2Exp("kubelet-pod-os-not-supported",
		"Pods that declare a Windows OS are bound to a Linux node; the "+
			"kubelet rejects each with PodOSNotSupported.",
		"deployment/shop/api", "notify", "node//n2").budget(3),
		"PodOSNotSupported", "Pod was rejected: Pod OS (windows) is not "+
			"supported on node OS (linux)")
}

// gap4aFeatureUnsupported: a Deployment asks for supplementalGroupsPolicy
// Strict; the runtime of n2 does not support it, the kubelet declines.
func gap4aFeatureUnsupported() scenario {
	e := g2Exp("kubelet-supplemental-groups-policy-unsupported",
		"A pod asks for supplementalGroupsPolicy Strict on a node whose "+
			"runtime lacks it; the kubelet rejects each pod and lists "+
			"the unsatisfied feature.",
		"deployment/shop/api", "notify").budget(3)
	e.Ambiguous = true
	return gap4aRejected(e, "SupplementalGroupsPolicyNotSupported",
		"Pod was rejected: SupplementalGroupsPolicy=Strict is not "+
			"supported by the node: the container runtime does not "+
			"report the feature")
}

// gap4aShutdownReject: a pod is bound to a node in the second it begins its
// shutdown; the kubelet rejects it and its replacement runs on another
// node within half a minute.
func gap4aShutdownReject() scenario {
	return scenario{
		expect: g2Quiet("kubelet-rejects-pod-node-shutting-down-quiet",
			"A pod bound to a node as it starts shutting down is "+
				"rejected by the kubelet; the replacement runs on "+
				"another node within half a minute."),
		build: func(c *cluster) {
			w := gap4aScaleUp(c)
			message := "Pod was rejected as the node is shutting down."
			pod := w.pod(2, "n1", g2Failed("NodeShutdown", message))
			c.create(pod)
			gapKubelet(c, pod, "NodeShutdown", message, 1)
			c.after(10 * time.Second)
			c.create(w.pod(3, "n2", startedNow, notReady))
			c.after(20 * time.Second)
			c.update(w.pod(3, "n2", startedNow))
			w.setReady(3)
			c.update(w.objects())
			c.after(10 * time.Minute)
		},
	}
}
