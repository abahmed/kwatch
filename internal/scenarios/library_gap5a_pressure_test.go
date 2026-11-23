package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gap5aPressureScenarios are nodes under memory, disk and PID pressure.
func gap5aPressureScenarios() []scenario {
	return []scenario{
		gap5aPageCacheEviction(), gap5aEvictionsInSuccession(),
		gap5aPressureFlaps(), gap5aExecProbeHangs(),
	}
}

// gap5aPageCacheEviction: a pod that streams large files keeps the file
// cache of its node full; the node reports memory pressure and evicts the
// pod again and again though its heap is small.
func gap5aPageCacheEviction() scenario {
	return scenario{
		expect: g2Exp("page-cache-memory-pressure-evicts-reader",
			"A pod that streams large files fills its node's page cache; "+
				"the node reports MemoryPressure and the kubelet evicts "+
				"that pod repeatedly.",
			"deployment/media/reader", "notify", "node//n1",
			"deployment/shop/orders", "deployment/shop/payments",
			"deployment/shop/cart").tail(10 * time.Minute),
		build: func(c *cluster) {
			gapNodeFleet(c)
			reader := c.deployment("media", "reader",
				"registry.example.com/reader:2", 1)
			c.list(reader.objects())
			c.list(reader.pod(0, "n1"))
			message := "The node was low on resource: memory. Threshold " +
				"quantity: 100Mi, available: 62Mi. Container app was " +
				"using 7340032Ki, request is 200Mi, has larger " +
				"consumption of memory."
			for round := range 4 {
				c.after(6 * time.Minute)
				g2NodeCondition(c, "n1", corev1.NodeMemoryPressure,
					corev1.ConditionTrue, "KubeletHasInsufficientMemory",
					"kubelet has insufficient memory available")
				c.update(reader.pod(round, "n1", evicted(message)))
				reader.setReady(0)
				c.update(reader.objects())
				c.after(time.Minute)
				g2NodeCondition(c, "n1", corev1.NodeMemoryPressure,
					corev1.ConditionFalse, "KubeletHasSufficientMemory",
					"kubelet has sufficient memory available")
				c.create(reader.pod(round+1, "n1", startedNow))
				reader.setReady(1)
				c.update(reader.objects())
			}
		},
	}
}

// gap5aEvictionsInSuccession: a node at its disk threshold evicts one pod
// after another, each freeing too little, five in six minutes.
func gap5aEvictionsInSuccession() scenario {
	return scenario{
		expect: g2Exp("evictions-in-succession-disk-threshold",
			"A node at its disk threshold evicts five pods of "+
				"different workloads in six minutes because each "+
				"frees too little; DiskPressure persists.",
			"node//n1", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"orders", "payments",
				"cart", "search", "ledger"}, []string{"n1", "n2"})
			c.after(5 * time.Minute)
			g2NodeCondition(c, "n1", corev1.NodeDiskPressure,
				corev1.ConditionTrue, "KubeletHasDiskPressure",
				"kubelet has disk pressure")
			for _, w := range fleet {
				c.update(w.pod(0, "n1", evicted("The node was low on "+
					"resource: ephemeral-storage. Threshold quantity: "+
					"10%, available: 9%.")))
				w.setReady(1)
				c.update(w.objects())
				c.after(75 * time.Second)
			}
			c.after(5 * time.Minute)
		},
	}
}

// gap5aPressureFlaps: a node's memory pressure oscillates around its
// threshold, six times in half an hour, and its pressure taint appears and
// goes.
func gap5aPressureFlaps() scenario {
	return scenario{
		expect: g2Exp("node-pressure-condition-flaps",
			"A node's MemoryPressure condition and taint switch on and "+
				"off six times in half an hour while it stays Ready.",
			"node//n1", "digest", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			gapNodeFleet(c)
			for range 6 {
				c.after(2 * time.Minute)
				node := g2NodeCondition(c, "n1", corev1.NodeMemoryPressure,
					corev1.ConditionTrue, "KubeletHasInsufficientMemory",
					"kubelet has insufficient memory available")
				node.Spec.Taints = []corev1.Taint{{
					Key:    "node.kubernetes.io/memory-pressure",
					Effect: "NoSchedule"}}
				c.update(node)
				c.after(3 * time.Minute)
				node = g2NodeCondition(c, "n1", corev1.NodeMemoryPressure,
					corev1.ConditionFalse, "KubeletHasSufficientMemory",
					"kubelet has sufficient memory available")
				node.Spec.Taints = nil
				c.update(node)
			}
		},
	}
}

// gap5aExecProbeHangs: a release's exec liveness script never exits;
// each probe times out, the kubelet restarts the container, and the
// defunct processes it leaves later push the node to PID pressure.
func gap5aExecProbeHangs() scenario {
	return g2Release(g2Exp("release-exec-probe-hangs-pid-pressure",
		"A release adds an exec probe whose script never exits; probes "+
			"time out, the container restarts and the node reports "+
			"PIDPressure later.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].LivenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{
					Command: []string{"/probe.sh"}}},
				PeriodSeconds: 10, TimeoutSeconds: 5}
		},
		func(c *cluster, w *workload) {
			gapRounds(c, w, 6, 45*time.Second, func(round int32) podState {
				return crashLoop(137, "Error", "", round)
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "Unhealthy", "Liveness probe failed: "+
					"command timed out: \"/probe.sh\" timed out after 5s",
					round)
			})
			g2NodeCondition(c, "n1", corev1.NodePIDPressure,
				corev1.ConditionTrue, "KubeletHasInsufficientPID",
				"kubelet has insufficient PID available")
			c.after(5 * time.Minute)
		})
}
