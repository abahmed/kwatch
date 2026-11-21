package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gap3NodeScenarios are node problems a node problem detector reports as
// conditions and events: hung tasks, file system faults, hardware errors,
// restarting node agents, device leaks and corrupt runtime storage.
func gap3NodeScenarios() []scenario {
	return []scenario{
		gap3TaskHung(), gap3XfsShutdown(), gap3HardwareError(),
		gap3KubeletRestarts(), gap3KubeletSingleRestart(),
		gap3UnregisterNetdevice(), gap3CorruptImageLayer(),
	}
}

// gap3Fleet lists three workloads of the shop with a replica on each of
// the nodes n1, n2 and n3, and returns them.
func gap3Fleet(c *cluster) []*workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
		c.node("n3", "zone-a"))
	return nodeFleet(c, "shop", []string{"orders", "payments", "cart"},
		[]string{"n1", "n2", "n3"})
}

// gap3Stall makes replica idx of each workload, on node, fail its
// readiness probe for rounds rounds with message.
func gap3Stall(
	c *cluster, fleet []*workload, node string, idx int, message string,
	rounds int32,
) {
	for round := int32(1); round <= rounds; round++ {
		for _, w := range fleet {
			pod := w.pod(idx, node, notReady)
			c.update(pod)
			gapKubelet(c, pod, "Unhealthy", message, round)
			w.setReady(2)
			c.update(w.objects())
		}
		c.after(time.Minute)
	}
}

// gap3TaskHung: the node problem detector sees a kernel task blocked for
// more than two minutes on n2; the node is Ready but every pod there
// fails its readiness probe by timeout, while the replicas on n1 and n3
// answer.
func gap3TaskHung() scenario {
	return scenario{
		expect: g2Exp("node-kernel-task-hung",
			"A node problem detector reports a hung kernel task and "+
				"KernelDeadlock on a node that is still Ready; the pods "+
				"on it time out their probes, their replicas on other "+
				"nodes work.",
			"node//n2", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			fleet := gap3Fleet(c)
			c.after(2 * time.Minute)
			node := g2NodeCondition(c, "n2", "KernelDeadlock",
				corev1.ConditionTrue, "DockerHung", "task docker:7421 "+
					"blocked for more than 120 seconds.")
			g2Event(c, node, "Node", "TaskHung", "task docker:7421 "+
				"blocked for more than 120 seconds.", "kernel-monitor", 3)
			gap3Stall(c, fleet, "n2", 1, "Readiness probe failed: Get "+
				"\"http://10.244.2.9:8080/ready\": context deadline "+
				"exceeded (Client.Timeout exceeded while awaiting "+
				"headers)", 6)
		},
	}
}

// gap3XfsShutdown: the XFS file system of n3's data disk shuts down; the
// detector sets XfsShutdown and pods that write to local disk fail with
// I/O errors while the node stays Ready.
func gap3XfsShutdown() scenario {
	return scenario{
		expect: g2Exp("node-xfs-shutdown",
			"A node problem detector sets XfsShutdown on a node and "+
				"reports I/O errors; pods that write local disk there "+
				"crash with \"input/output error\", their replicas on "+
				"other nodes work.",
			"node//n3", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			fleet := gap3Fleet(c)
			c.after(2 * time.Minute)
			node := g2NodeCondition(c, "n3", "XfsShutdown",
				corev1.ConditionTrue, "XfsHasShutdown",
				"XFS has shutdown on /dev/nvme1n1")
			g2Event(c, node, "Node", "IOError", "Buffer I/O error on "+
				"dev nvme1n1, logical block 8842, lost async page write",
				"kernel-monitor", 5)
			message := "write /var/lib/app/state.db: input/output error"
			for round := int32(1); round <= 6; round++ {
				for _, w := range fleet {
					c.update(w.pod(2, "n3", crashLoop(1, "Error", message,
						round)))
					w.setReady(2)
					c.update(w.objects())
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap3HardwareError: the detector reports a fatal CPER hardware error and
// a kernel oops on n1; minutes later the node stops reporting.
func gap3HardwareError() scenario {
	return scenario{
		expect: g2Exp("node-hardware-error-before-loss",
			"A node reports a fatal hardware error and a kernel oops, "+
				"then stops posting status; its pods go unready.",
			"node//n1", "page", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(2 * time.Minute)
			node := g2NodeCondition(c, "n1", "CperHardwareErrorFatal",
				corev1.ConditionTrue, "CperHardwareErrorFatal",
				"CPER: fatal hardware error: memory controller uncorrected")
			g2Event(c, n1, "Node", "KernelOops", "BUG: unable to handle "+
				"kernel paging request at ffffc9000e2a3f80", "kernel-monitor",
				1)
			c.after(3 * time.Minute)
			nodeLose(c, node)
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(5 * time.Minute)
		},
	}
}

// gap3KubeletRestarts: the kubelet of n1 restarts every two minutes
// (systemd restarts it after a crash); the node stays Ready, but the
// pods of three workloads there blip unready and count restarts.
func gap3KubeletRestarts() scenario {
	return scenario{
		expect: g2Exp("node-kubelet-restarts-repeatedly",
			"A node's kubelet restarts every two minutes; the detector "+
				"sets FrequentKubeletRestart and the pods of three "+
				"workloads on it blip unready while the node stays "+
				"Ready.",
			"node//n1", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			_, fleet := gapNodeFleet(c)
			c.after(time.Minute)
			g2NodeCondition(c, "n1", "FrequentKubeletRestart",
				corev1.ConditionTrue, "FrequentKubeletRestart",
				"kubelet.service restarted 6 times in the last 20 minutes")
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					c.update(w.pod(0, "n1", gap2Restarted(round), notReady))
					w.setReady(1)
					c.update(w.objects())
				}
				c.after(20 * time.Second)
				for _, w := range fleet {
					c.update(w.pod(0, "n1", gap2Restarted(round)))
					w.setReady(2)
					c.update(w.objects())
				}
				c.after(100 * time.Second)
			}
		},
	}
}

// gap3KubeletSingleRestart: the kubelet of n1 is restarted once for a
// configuration change; the pods stay ready.
func gap3KubeletSingleRestart() scenario {
	return scenario{
		expect: g2Quiet("node-kubelet-single-restart-quiet",
			"A node's kubelet restarts once for a configuration "+
				"change; the node stays Ready and its pods stay ready."),
		build: func(c *cluster) {
			gapNodeFleet(c)
			c.after(5 * time.Minute)
			g2NodeCondition(c, "n1", "FrequentKubeletRestart",
				corev1.ConditionFalse, "NoFrequentKubeletRestart",
				"kubelet.service is functioning properly")
			c.after(10 * time.Minute)
		},
	}
}

// gap3UnregisterNetdevice: the kernel of n3 repeats "unregister_netdevice:
// waiting for veth to become free"; pods deleted there stay Terminating
// and replacement pods cannot get a network sandbox.
func gap3UnregisterNetdevice() scenario {
	return scenario{
		expect: g2Exp("node-unregister-netdevice-stuck",
			"A node's kernel cannot release network devices; the "+
				"detector sets FrequentUnregisterNetDevice, deleted pods "+
				"stay Terminating and new pods fail to create a "+
				"sandbox.",
			"node//n3", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, []string{"n1", "n3"})
			c.after(2 * time.Minute)
			g2NodeCondition(c, "n3", "FrequentUnregisterNetDevice",
				corev1.ConditionTrue, "UnregisterNetDevice",
				"unregister_netdevice: waiting for eth0 to become free. "+
					"Usage count = 1")
			message := "Failed to create pod sandbox: rpc error: code = " +
				"Unknown desc = failed to setup network for sandbox: " +
				"plugin type=\"cilium-cni\" failed (add): unable to " +
				"create endpoint: veth already exists"
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					old := w.pod(1, "n3", notReady)
					deleting(&old.ObjectMeta, c.now.Add(-time.Minute))
					c.update(old)
					pod := w.pod(2, "n3", startedNow,
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

// gap3CorruptImageLayer: the runtime's image store on n2 is corrupt; a
// new replica placed there fails to unpack the image's layers while the
// replica placed on n1 starts from the same image.
func gap3CorruptImageLayer() scenario {
	return scenario{
		expect: g2Exp("node-corrupt-image-layer",
			"A new replica on a node whose runtime image store is "+
				"corrupt fails to unpack the image (\"failed to register "+
				"layer\"); the same image starts on another node.",
			"node//n2", "notify", "deployment/shop/api",
			"registry//registry.example.com").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:4", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(5 * time.Minute)
			*w.deployment.Spec.Replicas = 4
			w.setReady(2)
			c.update(w.objects())
			c.create(w.pod(2, "n1", startedNow, notReady))
			c.after(30 * time.Second)
			c.update(w.pod(2, "n1", startedNow))
			message := "Failed to pull image \"registry.example.com/api:4" +
				"\": failed to register layer: ApplyLayer exit status 1 " +
				"stdout:  stderr: open /var/lib/containerd/io.containerd." +
				"snapshotter.v1.overlayfs/snapshots/412/fs/usr/lib/" +
				"libssl.so.3: input/output error"
			created := c.now
			for round := int32(1); round <= 6; round++ {
				reason := "ErrImagePull"
				if round%2 == 0 {
					reason = "ImagePullBackOff"
				}
				pod := w.pod(3, "n2", createdAt(created),
					gapPending(waiting(reason, message)))
				c.update(pod)
				gapKubelet(c, pod, "Failed", message, round)
				c.after(time.Minute)
			}
		},
	}
}
