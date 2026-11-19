package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gap3MiscAScenarios are nodes, volumes and pods whose state the cluster
// reports through conditions and events of the controllers that own them.
func gap3MiscAScenarios() []scenario {
	return []scenario{
		gap3LoadBalancerNoNodes(), gap3CIDRNotAvailable(), gap3ClaimLost(),
		gap3VolumeMismatch(), gap3NodeLostMultiAttach(),
		gap3NodeShutdown(), gap3NodeShutdownReplaced(), gap3ResizePending(),
		gap3SchedulingGated(), gap3SchedulingGateRemoved(),
	}
}

// gap3LoadBalancerNoNodes: every node carries the label that excludes it
// from external load balancers; the Service controller has no target.
func gap3LoadBalancerNoNodes() scenario {
	return scenario{
		expect: g2Exp("loadbalancer-service-no-eligible-nodes",
			"A LoadBalancer Service has no node to register: every "+
				"node is excluded from external load balancers; the "+
				"Service controller reports UnAvailableLoadBalancer.",
			"unknown", "digest", "deployment/shop/web").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			for _, name := range []string{"n1", "n2"} {
				node := c.node(name, "zone-a")
				node.Labels["node.kubernetes.io/exclude-from-external-"+
					"load-balancers"] = ""
				c.list(node)
			}
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			svc := clusterService(c, "shop", "web", 443)
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			c.list(svc, clusterSlice(c, "shop", "web", w.pod(0, "n1"),
				w.pod(1, "n2")))
			for m := int32(1); m <= 4; m++ {
				c.after(5 * time.Minute)
				g2Event(c, svc, "Service", "UnAvailableLoadBalancer",
					"There are no available nodes for LoadBalancer",
					"service-controller", m)
			}
		},
	}
}

// gap3CIDRNotAvailable: the seventeenth node of a /20 cluster range joins
// without a pod CIDR; pods placed on it cannot set up their network.
func gap3CIDRNotAvailable() scenario {
	return scenario{
		expect: g2Exp("node-pod-cidr-not-available",
			"A node that joined after the cluster's pod range was used "+
				"up has no pod CIDR; the controller reports "+
				"CIDRNotAvailable and pods placed on it stay in "+
				"ContainerCreating.",
			"node//n4", "notify", "deployment/shop/web",
			"deployment/billing/invoices").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n4", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices"},
				[]string{"n1"})
			c.after(5 * time.Minute)
			node := g2NodeCondition(c, "n4", corev1.NodeNetworkUnavailable,
				corev1.ConditionTrue, "NoRouteCreated",
				"Node created without a route")
			g2Event(c, node, "Node", "CIDRNotAvailable", "Node "+
				node.Name+" status is now: CIDRNotAvailable",
				"cidrAllocator", 1)
			message := "Failed to create pod sandbox: rpc error: code = " +
				"Unknown desc = failed to setup network for sandbox: " +
				"plugin type=\"bridge\" failed (add): failed to " +
				"allocate for range 0: no pod CIDR for the node"
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
			}
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n4", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedCreatePodSandBox", message,
						round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap3ClaimLost: someone deletes the PersistentVolume object under a
// bound claim; the claim goes Lost and the pod that restarts cannot mount
// it.
func gap3ClaimLost() scenario {
	return scenario{
		expect: g2Exp("pvc-lost-volume-deleted",
			"A PersistentVolume is deleted while its claim is bound; "+
				"the claim becomes Lost (ClaimLost) and the pod that "+
				"uses it cannot start.",
			"persistentvolumeclaim/shop/data", "notify", "node//n1",
			"deployment/shop/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			claim := storageBoundClaim(c, "shop", "data", "fast", "20Gi",
				"pv-data")
			pv := storageVolume(c, claim, "pv-data")
			c.list(scheduleClass(c, "fast", "Immediate"), pv, claim)
			w := c.deployment("shop", "db", "registry.example.com/db:5", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(10 * time.Minute)
			c.remove(pv)
			lost := last(c, claim)
			lost.Status.Phase = corev1.ClaimLost
			c.update(lost)
			g2Event(c, lost, "PersistentVolumeClaim", "ClaimLost", "Bound "+
				"claim has lost its PersistentVolume. Data on the "+
				"volume is lost!", "persistentvolume-controller", 1)
			c.after(time.Minute)
			c.remove(w.pod(0, "n1"))
			w.setReady(0)
			c.update(w.objects())
			for round := int32(1); round <= 5; round++ {
				pod := w.pod(1, "n1", startedNow,
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				gapKubelet(c, pod, "FailedMount", "Unable to attach or "+
					"mount volumes: unmounted volumes=[data], unattached "+
					"volumes=[data kube-api-access]: timed out waiting "+
					"for the condition", round)
				c.after(time.Minute)
			}
		},
	}
}

// gap3VolumeMismatch: a claim names the static volume nfs-pv-5g but asks
// for 10Gi; the controller cannot bind them.
func gap3VolumeMismatch() scenario {
	return scenario{
		expect: g2Exp("pvc-static-volume-too-small",
			"A claim pre-bound to a static volume asks for more "+
				"capacity than the volume has; it stays Pending with "+
				"VolumeMismatch.",
			"persistentvolumeclaim/shop/data-nfs", "notify", "node//n1",
			"deployment/shop/reports").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			bound := storageBoundClaim(c, "shop", "old-claim", "", "5Gi",
				"nfs-pv-5g")
			pv := storageVolume(c, bound, "nfs-pv-5g")
			pv.Spec.ClaimRef = nil
			pv.Status.Phase = corev1.VolumeAvailable
			c.list(pv)
			claim := scheduleClaim(c, "shop", "data-nfs", "")
			claim.Spec.VolumeName = c.n("nfs-pv-5g")
			c.create(claim)
			w := c.deployment("shop", "reports",
				"registry.example.com/reports:2", 1)
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: pod has unbound " +
				"immediate PersistentVolumeClaims. preemption: 0/1 " +
				"nodes are available: 1 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				g2Event(c, claim, "PersistentVolumeClaim", "VolumeMismatch",
					"Volume's size is smaller than requested or volume's "+
						"class does not match with claim",
					"persistentvolume-controller", m)
			}
		},
	}
}

// gap3NodeLostMultiAttach: n1 stops reporting while it holds the
// ReadWriteOnce disk of a database; the replacement pod on n2 waits on
// Multi-Attach until the controller detaches the disk.
func gap3NodeLostMultiAttach() scenario {
	return scenario{
		expect: g2Exp("node-lost-rwo-volume-multi-attach",
			"A node that holds a ReadWriteOnce disk stops reporting; "+
				"the database's replacement pod waits on a Multi-Attach "+
				"error.",
			"node//n1", "page", "deployment/shop/db",
			"persistentvolumeclaim/shop/data").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			claim := storageBoundClaim(c, "shop", "data", "fast", "20Gi",
				"pv-data")
			c.list(scheduleClass(c, "fast", "Immediate"),
				storageVolume(c, claim, "pv-data"), claim)
			w := c.deployment("shop", "db", "registry.example.com/db:5", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(5 * time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			c.update(w.pod(0, "n1", notReady))
			w.setReady(0)
			c.update(w.objects())
			c.after(5 * time.Minute)
			old := w.pod(0, "n1", notReady)
			deleting(&old.ObjectMeta, c.now)
			c.update(old)
			for round := int32(1); round <= 5; round++ {
				pod := w.pod(1, "n2", startedNow,
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				gapKubelet(c, pod, "FailedAttachVolume", "Multi-Attach "+
					"error for volume \""+c.n("pv-data")+"\" Volume is "+
					"already exclusively attached to one node and can't "+
					"be attached to another", round)
				c.after(time.Minute)
			}
		},
	}
}

// gap3Shutdown reboots n1 for patching: its pods are terminated by the
// kubelet's node shutdown handling and end Failed. With bare a pod that
// no controller owns is among them.
func gap3Shutdown(c *cluster, bare bool) {
	_, fleet := gapNodeFleet(c)
	message := "Pod was terminated in response to imminent node shutdown."
	var tools *corev1.Pod
	if bare {
		tools = fleet[0].pod(0, "n1")
		tools.Name, tools.UID, tools.OwnerReferences = "tools-shell",
			"tools-shell", nil
		c.list(tools)
	}
	c.after(5 * time.Minute)
	for _, w := range fleet {
		pod := w.pod(0, "n1", g2Failed("Terminated", message))
		c.update(pod)
		gapKubelet(c, pod, "NodeShutdown", message, 1)
		w.setReady(1)
		c.update(w.objects())
	}
	if bare {
		failed := tools.DeepCopy()
		g2Failed("Terminated", message)(c, failed)
		c.update(failed)
	}
	c.after(30 * time.Second)
	for i, w := range fleet {
		c.remove(w.pod(0, "n1"))
		c.create(w.pod(2+i, "n2", startedNow))
		w.setReady(2)
		c.update(w.objects())
	}
	c.after(10 * time.Minute)
}

// gap3NodeShutdown: a bare pod and the pods of three Deployments are
// terminated by a node reboot; the Deployments replace theirs, the bare
// pod is gone for good.
func gap3NodeShutdown() scenario {
	return scenario{
		expect: g2Exp("node-shutdown-terminates-bare-pod",
			"A node is rebooted for patching; its pods end Failed with "+
				"\"terminated in response to imminent node shutdown\"; "+
				"Deployments replace theirs but a bare pod is not "+
				"restarted.", "node//n1", "digest", "pod/shop/tools-shell",
			"deployment/shop/orders"),
		build: func(c *cluster) { gap3Shutdown(c, true) },
	}
}

// gap3NodeShutdownReplaced: the same reboot with only Deployment pods.
func gap3NodeShutdownReplaced() scenario {
	return scenario{
		expect: g2Quiet("node-shutdown-pods-replaced-quiet",
			"A node is rebooted for patching; its Deployment pods end "+
				"Failed with the node shutdown reason and are replaced "+
				"on another node within a minute."),
		build: func(c *cluster) { gap3Shutdown(c, false) },
	}
}

// gap3ResizePending: an operator raises a pod's CPU in place; the node
// has no room, the resize is Deferred and the pod keeps its old size.
func gap3ResizePending() scenario {
	return scenario{
		expect: g2Exp("pod-resize-deferred-no-room",
			"An in-place resize of a pod's CPU stays Pending "+
				"(PodResizePending, Deferred) because its node has no "+
				"room.", "node//n1", "notify", "deployment/shop/api").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			message := "Node didn't have enough capacity: cpu, requested: " +
				"6000, capacity: 4000"
			for m := int32(1); m <= 5; m++ {
				pod := w.pod(0, "n1")
				pod.Status.Conditions = append(pod.Status.Conditions,
					corev1.PodCondition{Type: "PodResizePending",
						Status: corev1.ConditionTrue, Reason: "Deferred",
						Message: message})
				c.update(pod)
				gapKubelet(c, pod, "ResizeDeferred", message, m)
				c.after(2 * time.Minute)
			}
		},
	}
}

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
